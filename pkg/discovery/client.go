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
	"sync"
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

	clientNew       = 0
	clientSched     = 1
	clientTimedout  = 2
	clientCanceled  = 3
	clientConnected = 4
)

type discoveryClient struct {
	name     string
	address  string
	waitChan chan struct{}
	cancel   context.CancelFunc
	status   int
}

var discoveryServices map[string]*discoveryClient

// TODO use a set of per entry mutexes if need be
var discoveryMutex sync.Mutex

func newServiceBinding(in *pb.ExposedServicesMessages_ExposedService, name string) *mmv1.ServiceBinding {
	return &mmv1.ServiceBinding{
		TypeMeta: metav1.TypeMeta{
			Kind: "ServiceBinding",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      in.Name, //strings.ToLower(strings.Replace(name, " ", "", -1)),
			Namespace: "default",
		},
		Spec: mmv1.ServiceBindingSpec{
			Name:                  in.Name,
			Namespace:             "default",
			Port:                  in.Port,
			MeshFedConfigSelector: in.MeshFedConfigSelector,
			Endpoints:             in.Endpoints,
		},
	}
}

func createServiceBindings(sbr *controllers.ServiceBindingReconciler, in *pb.ExposedServicesMessages) error {
	for _, v := range in.GetExposedServices() {
		nv := newServiceBinding(v, in.GetName())
		_, err := controllerutil.CreateOrUpdate(context.Background(), sbr.Client, nv, func() error { return nil })
		if err != nil {
			return err
		}

	}
	return nil
}

// ClientStarter starting the clients for remote discovery servers
func ClientStarter(ctx context.Context, sbr *controllers.ServiceBindingReconciler,
	svcr *controllers.ServiceReconciler, discoveryChannel chan controllers.DiscoveryServer) {
	discoveryServices = make(map[string]*discoveryClient)
	go clientMonitor(ctx, sbr, svcr)
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
						status: clientNew,
					}
					discoveryMutex.Lock()
					discoveryServices[svc.Name] = &dc
					discoveryMutex.Unlock()
				} else {
					// This is in response to an update to service
					// If address has changed, kill the existing client
					// and start a new one
					// TODO: using a version entry, we could delegate this to the mon
					if discoveryServices[svc.Name].address != svc.Address {
						discoveryServices[svc.Name].cancel()
						if discoveryServices[svc.Name].status == clientConnected {
							discoveryServices[svc.Name].waitChan <- struct{}{}
						}
						discoveryMutex.Lock()
						delete(discoveryServices, svc.Name)
						discoveryMutex.Unlock()

						waitc := make(chan struct{})
						dc := discoveryClient{
							name:     svc.Name,
							address:  svc.Address,
							waitChan: waitc,
							// cancel:   cancel, // to be set when starting client
							status: clientNew,
						}
						discoveryServices[svc.Name] = &dc
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
					discoveryMutex.Lock()
					delete(discoveryServices, svc.Name)
					discoveryMutex.Unlock()
				}
			}
		}
	}
}

func clientMonitor(ctx context.Context, sbr *controllers.ServiceBindingReconciler, svcr *controllers.ServiceReconciler) {
	for {
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
					discoveryMutex.Lock()
					switch v.status {
					case clientNew:
						v.status = clientSched
						go Client(ctx, sbr, v)
					case clientTimedout:
						// if svc still exists, reschedule client
						delete(discoveryServices, k)

						waitc := make(chan struct{})
						dc := discoveryClient{
							name:     v.name,
							address:  v.address,
							waitChan: waitc,
							// cancel:   cancel,
							status: clientNew,
						}
						discoveryServices[k] = &dc
					case clientCanceled:
						// Not dealing with cancels here yet.
						delete(discoveryServices, k)
					case clientConnected:
						// do nothing
					case clientSched:
						// do nothing
					}
					discoveryMutex.Unlock()
				} else {
					// svc has been deleted, (if already connected) stop the client
					v.cancel()
					v.waitChan <- struct{}{}
					discoveryMutex.Lock()
					delete(discoveryServices, k)
					discoveryMutex.Unlock()
				}
			} else {
				// shouldn't get here.
				log.Warnf("Incorrect key for a discovery server. Deleting it: key: %v value: %v", k, v)
				discoveryMutex.Lock()
				delete(discoveryServices, k)
				discoveryMutex.Unlock()
			}
		}
		time.Sleep(connMonitorSeconds * time.Second)
	}
}

// Client is the ESDS grpc client
func Client(ctx context.Context, sbr *controllers.ServiceBindingReconciler, disc *discoveryClient) {
	// Set up a connection to the server.
	var err error
	var conn *grpc.ClientConn = nil

	discoveryClientCtx, cancel := context.WithTimeout(ctx, connTimeoutSeconds*time.Second)
	disc.cancel = cancel
	conn, err = grpc.DialContext(discoveryClientCtx, disc.address, grpc.WithInsecure(), grpc.WithBlock())

	if err != nil {
		log.Infof("Did not connect to %v. Error: %v", disc.address, err)
		discoveryMutex.Lock()
		if strings.Contains(err.Error(), "context deadline exceeded") {
			disc.status = clientTimedout
		} else {
			disc.status = clientCanceled
		}
		discoveryMutex.Unlock()
		return
	}

	defer conn.Close()

	c := pb.NewESDSClient(conn)
	stream, _ := c.ExposedServicesDiscovery(ctx)
	waitc := disc.waitChan
	discoveryMutex.Lock()
	disc.status = clientConnected
	discoveryMutex.Unlock()

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
			createServiceBindings(sbr, in)
			log.Infof("Processd ESDA Discovery message")
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
