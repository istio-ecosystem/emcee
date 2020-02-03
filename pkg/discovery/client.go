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
	"fmt"
	"io"
	"time"

	mmv1 "github.com/istio-ecosystem/emcee/api/v1"
	"github.com/istio-ecosystem/emcee/controllers"
	pb "github.com/istio-ecosystem/emcee/pkg/discovery/api"
	"google.golang.org/grpc"
	"istio.io/pkg/log"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	defaultName = "Server x100"
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
	status   bool
}

var discoveryServices map[string]*discoveryClient

func ClientStarter(ctx context.Context, sbr *controllers.ServiceBindingReconciler, discoveryChannel chan controllers.DiscoveryServer) {
	discoveryServices = make(map[string]*discoveryClient)
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
					discoveryClientCtx, cancel := context.WithCancel(context.Background())
					dc := discoveryClient{
						name:     getName(svc.Address),
						address:  svc.Address,
						waitChan: waitc,
						cancel:   cancel,
					}
					discoveryServices[svc.Name] = &dc
					go Client(discoveryClientCtx, sbr, &dc, waitc)
				} else {
					// This is in response to an update to service
					// If address has changed, kill the existing client
					// and start a new one
					if discoveryServices[svc.Name].address != svc.Address {
						discoveryServices[svc.Name].cancel()
						if discoveryServices[svc.Name].status == true {
							discoveryServices[svc.Name].waitChan <- struct{}{}
						}
						waitc := make(chan struct{})
						discoveryClientCtx, cancel := context.WithCancel(context.Background())
						dc := discoveryServices[svc.Name]
						dc.address = svc.Address
						dc.waitChan = waitc
						dc.cancel = cancel
						dc.status = false
						go Client(discoveryClientCtx, sbr, dc, waitc)
					}
				}
			} else if svc.Operation == "D" {
				// This is in response to a deletion of a service
				// if exists, delet the client
				if ok {
					discoveryServices[svc.Name].cancel()
					if discoveryServices[svc.Name].status == true {
						discoveryServices[svc.Name].waitChan <- struct{}{}
					}
					delete(discoveryServices, svc.Name)
				}
			}
		default:
			for k, v := range discoveryServices {
				if v.status == "timedout" {

				}

			}
		}
	}
}

// Client is the ESDS grpc client
func Client(ctx context.Context, sbr *controllers.ServiceBindingReconciler, disc *discoveryClient, delChan chan struct{}) {
	// Set up a connection to the server.
	var err error
	var conn *grpc.ClientConn = nil

	conn, err = grpc.DialContext(ctx, disc.address, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		return
	}

	defer conn.Close()

	disc.status = true
	c := pb.NewESDSClient(conn)
	stream, _ := c.ExposedServicesDiscovery(context.Background())
	waitc := disc.waitChan
	go func() {
		for {
			in, err := stream.Recv()
			if err == io.EOF {
				// read done.
				close(waitc)
				return
			}
			if err != nil {
				log.Fatalf("Failed to receive a note : %v", err)
			}
			log.Infof("Received ESDA Discovery message: <%v>", in)
			createServiceBindings(sbr, in)
			log.Infof("Processd ESDA Discovery message")
		}
	}()

	var note pb.ExposedServicesMessages
	note.Name = "Request from clien"

	select {
	case <-waitc:
		stream.CloseSend()
		return
	case <-delChan:
		stream.CloseSend()
		waitc <- struct{}{}
		return
	default:
		if err := stream.Send(&note); err != nil {
			log.Fatalf("Failed to send a note: %v", err)
		}
		time.Sleep(3 * time.Second)
	}
}

func getName(name string) string {
	return fmt.Sprintf("Request from Client <%s> ", name)
}
