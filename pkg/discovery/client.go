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
	defaultName     = "Server x100"
	clientSched     = 1
	clientTimedout  = 2
	clientCanceled  = 3
	clientConnected = 4
)

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

type discoveryClient struct {
	name     string
	address  string
	waitChan chan struct{}
	cancel   context.CancelFunc
	status   int
}

var discoveryServices map[string]*discoveryClient

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
					discoveryClientCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
					dc := discoveryClient{
						name:     svc.Name,
						address:  svc.Address,
						waitChan: waitc,
						cancel:   cancel,
						status:   clientSched,
					}
					discoveryServices[svc.Name] = &dc
					go Client(discoveryClientCtx, sbr, &dc)
				} else {
					// This is in response to an update to service
					// If address has changed, kill the existing client
					// and start a new one
					if discoveryServices[svc.Name].address != svc.Address {
						discoveryServices[svc.Name].cancel()
						if discoveryServices[svc.Name].status == clientConnected {
							discoveryServices[svc.Name].waitChan <- struct{}{}
						}
						delete(discoveryServices, svc.Name)

						waitc := make(chan struct{})
						discoveryClientCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
						dc := discoveryClient{
							name:     svc.Name,
							address:  svc.Address,
							waitChan: waitc,
							cancel:   cancel,
							status:   clientSched,
						}
						discoveryServices[svc.Name] = &dc
						go Client(discoveryClientCtx, sbr, &dc)
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
					switch v.status {
					case clientTimedout:
						// if svc still exists, reschedule client
						waitc := make(chan struct{})
						discoveryClientCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
						dc := discoveryClient{
							name:     v.name,
							address:  v.address,
							waitChan: waitc,
							cancel:   cancel,
							status:   clientSched,
						}
						delete(discoveryServices, k)
						discoveryServices[k] = &dc
						go Client(discoveryClientCtx, sbr, &dc)
					case clientCanceled:
						// TODO why here?
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
				delete(discoveryServices, k)
			}
		}
		time.Sleep(3 * time.Second)
	}
}

// Client is the ESDS grpc client
func Client(ctx context.Context, sbr *controllers.ServiceBindingReconciler, disc *discoveryClient) {
	// Set up a connection to the server.
	var err error
	var conn *grpc.ClientConn = nil

	conn, err = grpc.DialContext(ctx, disc.address, grpc.WithInsecure(), grpc.WithBlock())
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
	stream, _ := c.ExposedServicesDiscovery(context.Background())
	disc.status = clientConnected
	waitc := disc.waitChan

	var note pb.ExposedServicesMessages
	note.Name = "Request from clien"

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
			time.Sleep(10 * time.Second)
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
