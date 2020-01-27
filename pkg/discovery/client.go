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
		or, err := controllerutil.CreateOrUpdate(context.Background(), sbr.Client, nv, func() error { return nil })
		log.Infof("********************** 17 %v ---> %v <--- %v", nv, or, err)
		if err != nil {
			return err
		}

	}
	return nil
}

// Client is the ESDS grpc client
func Client(sbr *controllers.ServiceBindingReconciler, address *string) {
	// Set up a connection to the server.
	var err error
	var conn *grpc.ClientConn = nil
	for {
		conn, err = grpc.Dial(*address, grpc.WithInsecure(), grpc.WithBlock())
		if err != nil {
			log.Infof("Could not connect to Discovery server. Will try again.\n")
			time.Sleep(time.Second)
		} else {
			log.Infof("Connected to Discovery server.\n")
			break
		}
	}
	defer conn.Close()

	c := pb.NewESDSClient(conn)
	stream, err := c.ExposedServicesDiscovery(context.Background())
	waitc := make(chan struct{})
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
	note.Name = "Yoyo"
	for i := 0; i < 10000; {
		if err := stream.Send(&note); err != nil {
			log.Fatalf("Failed to send a note: %v", err)
		}
		time.Sleep(3 * time.Second)
		i++
	}
	stream.CloseSend()
	<-waitc
}
