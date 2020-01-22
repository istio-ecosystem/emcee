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
	"log"
	"time"

	"github.com/istio-ecosystem/emcee/controllers"
	pb "github.com/istio-ecosystem/emcee/pkg/discovery/api"
	"google.golang.org/grpc"
)

const (
	address     = "localhost:50051"
	defaultName = "Server x100"
)

// Client is the ESDS grpc client
func Client(sbr *controllers.ServiceBindingReconciler) {
	// Set up a connection to the server.
	conn, err := grpc.Dial(address, grpc.WithInsecure())
	if err != nil {
		log.Fatalf("did not connect: %v", err)
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
			log.Printf("Got message -**************** (%v)", in)
		}
	}()

	var note pb.ExposedServicesMessages
	note.Name = "Yoyo"
	for i := 0; i < 10; {
		if err := stream.Send(&note); err != nil {
			log.Printf("Requesting iter -=============== (%d)", i)
			log.Fatalf("Failed to send a note: %v", err)
		}
		time.Sleep(3 * time.Second)
		i++
	}
	stream.CloseSend()
	<-waitc
}
