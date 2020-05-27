/*
 *
 * Copyright 2015 gRPC authors.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 */

package discovery

import (
	"context"
	"errors"
	"io"
	"strings"
	"time"

	mmv1 "github.com/istio-ecosystem/emcee/api/v1"
	"github.com/istio-ecosystem/emcee/controllers"
	pb "github.com/istio-ecosystem/emcee/pkg/discovery/api"
	"google.golang.org/grpc"
	"istio.io/pkg/log"
	k8sapi "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	defaultName = "Server"

	connTimeoutSeconds = 10
	connMonitorSeconds = 3

	clientSched     = 1
	clientTimedout  = 2
	clientCanceled  = 3
	clientConnected = 4
)

type discoveryClient struct {
	name               string
	address            string
	waitChan           chan struct{}
	cancel             context.CancelFunc
	status             int
	discoveredServices map[string]int
}

var discoveryServices map[string]*discoveryClient

const (
	DEFAULT_NAMESPACE = "default" // TODO  generalize
	CLEAR             = 0
	CREATED           = 1
)

func newServiceBinding(in *pb.ExposedServicesMessages_ExposedService) *mmv1.ServiceBinding {
	var newName, newNamespace string
	s := strings.Split(in.Name, "/")
	if len(s) == 2 {
		newNamespace = s[0]
		newName = s[1]
	} else {
		newNamespace = DEFAULT_NAMESPACE
		newName = s[0]
	}

	return &mmv1.ServiceBinding{
		TypeMeta: metav1.TypeMeta{
			Kind: "ServiceBinding",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      newName,
			Namespace: newNamespace,
		},
		Spec: mmv1.ServiceBindingSpec{
			Name:                  newName,
			Namespace:             newNamespace,
			Port:                  in.Port,
			MeshFedConfigSelector: in.MeshFedConfigSelector,
			Endpoints:             in.Endpoints,
			// TODO Alias: in.Alias, // This is the alias on the binding side
		},
	}
}

func createServiceBindings(sbr *controllers.ServiceBindingReconciler, in *pb.ExposedServicesMessages,
	disc *discoveryClient) error {
	for k := range disc.discoveredServices {
		disc.discoveredServices[k] = CLEAR
	}

	for _, v := range in.GetExposedServices() {
		goalNv := newServiceBinding(v)
		nv := &mmv1.ServiceBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      goalNv.ObjectMeta.Name,
				Namespace: goalNv.ObjectMeta.Namespace,
			},
		}
		_, err := controllerutil.CreateOrUpdate(context.Background(), sbr.Client, nv, func() error {
			nv.ObjectMeta.Labels = goalNv.Labels
			nv.ObjectMeta.OwnerReferences = goalNv.ObjectMeta.OwnerReferences
			nv.Spec = goalNv.Spec
			return nil
		})
		if err != nil {
			return err
		}
		disc.discoveredServices[v.GetName()] = CREATED

	}

	for k := range disc.discoveredServices {
		if disc.discoveredServices[k] == CLEAR {
			var newName, newNamespace string
			s := strings.Split(k, "/")
			if len(s) == 2 {
				newNamespace = s[0]
				newName = s[1]
			} else {
				newNamespace = DEFAULT_NAMESPACE
				newName = s[0]
			}

			var binding mmv1.ServiceBinding
			nsn := types.NamespacedName{
				Name:      newName,
				Namespace: newNamespace,
			}
			if err := sbr.Client.Get(context.Background(), nsn, &binding); err == nil {
				sbr.Client.Delete(context.Background(), &binding)
			} else {
				log.Warnf("error in cleanup of deleted discovered service: %v", err)
			}
			delete(disc.discoveredServices, k)
		}
	}
	return nil
}

// ClientStarter starting the clients for remote discovery servers
func ClientStarter(ctx context.Context, sbr *controllers.ServiceBindingReconciler,
	svcr *controllers.ServiceReconciler, discoveryChannel chan controllers.DiscoveryServer) {
	discoveryServices = make(map[string]*discoveryClient)

	monitor := time.NewTicker(connMonitorSeconds * time.Millisecond)
	for {
		select {
		case svc := <-discoveryChannel:
			_, ok := discoveryServices[svc.Name]
			if svc.Operation == "U" {
				// This is in response to either a new or an existing service
				if !ok {
					// This is in response to a new service
					// Create the client for it
					waitc := make(chan struct{})
					dc := discoveryClient{
						name:     svc.Name,
						address:  svc.Address,
						waitChan: waitc,
						//cancel:   cancel, // to be set when starting client
						status: clientSched,
					}
					discoveryServices[svc.Name] = &dc
					discoveryServices[svc.Name].discoveredServices = make(map[string]int)
					go client(ctx, sbr, &dc)
				} else {
					// This is in response to an update to service
					// If address has changed, kill the existing client and start a new one
					// TODO: using a versioned entry, we could delegate this to the mon
					if discoveryServices[svc.Name].address != svc.Address {
						discoveryServices[svc.Name].cancel()
						if discoveryServices[svc.Name].status == clientConnected {
							discoveryServices[svc.Name].waitChan <- struct{}{}
						}
						delete(discoveryServices, svc.Name)
						waitc := make(chan struct{})
						dc := discoveryClient{
							name:     svc.Name,
							address:  svc.Address,
							waitChan: waitc,
							// cancel:   cancel, // to be set when starting client
							status: clientSched,
						}
						discoveryServices[svc.Name] = &dc
						go client(ctx, sbr, &dc)
					}

				}
			} else if svc.Operation == "D" {
				// This is in response to a deletion of a service
				// if exists, delet the client
				if ok {
					discoveryServices[svc.Name].cancel()
					if discoveryServices[svc.Name].status == clientConnected {
						discoveryServices[svc.Name].waitChan <- struct{}{}
					}
					delete(discoveryServices, svc.Name)
				}
			}
		case <-monitor.C:
			for k, v := range discoveryServices {
				var oldsvc k8sapi.Service
				ns, n, err := getNamespceAndName(v.name)
				if err == nil {
					key := types.NamespacedName{
						Namespace: ns,
						Name:      n,
					}
					err := svcr.Get(context.Background(), key, &oldsvc)
					if err == nil {
						switch v.status {
						case clientTimedout:
							// if svc still exists, reschedule client
							delete(discoveryServices, k)

							waitc := make(chan struct{})
							dc := discoveryClient{
								name:     v.name,
								address:  v.address,
								waitChan: waitc,
								// cancel:   cancel,
								status: clientSched,
							}
							discoveryServices[k] = &dc
							go client(ctx, sbr, &dc)
						case clientCanceled:
							// Not dealing with cancels here yet.
							delete(discoveryServices, k)
						case clientConnected:
							// do nothing
						case clientSched:
							// do nothing
						}
					} else {
						// svc has been deleted, (if already connected) stop the client
						v.cancel()
						v.waitChan <- struct{}{}
						delete(discoveryServices, k)
					}
				} else {
					// shouldn't get here.
					log.Warnf("Incorrect key for a discovery server. Deleting it: key: %v value: %v", k, v)
					delete(discoveryServices, k)
				}
			}
		}
	}
}

// Client is the ESDS grpc client
func client(ctx context.Context, sbr *controllers.ServiceBindingReconciler, disc *discoveryClient) {
	// Set up a connection to the server.
	var err error
	var conn *grpc.ClientConn = nil

	discoveryClientCtx, cancel := context.WithTimeout(ctx, connTimeoutSeconds*time.Second)
	disc.cancel = cancel
	conn, err = grpc.DialContext(discoveryClientCtx, disc.address, grpc.WithInsecure(), grpc.WithBlock())

	if err != nil {
		log.Infof("Did not connect to %v. Error: %v", disc.address, err)
		if strings.Contains(err.Error(), "context deadline exceeded") {
			disc.status = clientTimedout
		} else {
			disc.status = clientCanceled
		}
		return
	}
	defer conn.Close()

	c := pb.NewESDSClient(conn)
	stream, _ := c.ExposedServicesDiscovery(ctx)
	waitc := disc.waitChan
	disc.status = clientConnected

	var note pb.ExposedServicesMessages
	note.Name = "Request from client"

	go func() {
		for {
			in, err := stream.Recv()
			if err == io.EOF {
				// read done.
				close(waitc)
				return
			}
			if err != nil {
				log.Warnf("Failed to receive a note : %v", err)
				return
			}
			log.Infof("Received ESDA Discovery message: <%v>", in)
			createServiceBindings(sbr, in, disc)
			log.Infof("Processed ESDA Discovery message")
		}
	}()

	go func() {
		for {
			if err := stream.Send(&note); err != nil {
				log.Warnf("Failed to send a note: %v", err)
				return
			}
			time.Sleep(connTimeoutSeconds * time.Second)
		}
	}()

	select {
	case <-waitc:
		stream.CloseSend()
		return
	}
}

func getNamespceAndName(name string) (string, string, error) {
	var err error
	s := strings.Split(name, "/")
	if len(s) != 2 {
		err = errors.New("Bad namespaces name")
		return "", "", err
	}
	return s[0], s[1], err
}
