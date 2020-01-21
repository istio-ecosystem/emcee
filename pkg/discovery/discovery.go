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

//go:generate protoc -I ../banix --go_out=plugins=grpc:../banix ../banix/banix.proto

package discovery

import (
	"context"
	"log"
	"net"
	"time"

	mmv1 "github.com/istio-ecosystem/emcee/api/v1"
	"github.com/istio-ecosystem/emcee/controllers"
	pb "github.com/istio-ecosystem/emcee/pkg/discovery/api"
	"google.golang.org/grpc"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/reflection"
)

const (
	port = ":50051"
)

var seReconciler *controllers.ServiceExpositionReconciler

// server is used to implement Exposed Services Discovery Service.
type server struct{}

// SayHello implements ESDS server
func (s *server) ExposedServicesDiscovery(ctx context.Context, in *pb.ExposedServicesRequest) (*pb.ExposedServicesReply, error) {
	var list mmv1.ServiceExpositionList
	peer, ok := peer.FromContext(ctx)
	log.Printf("====<< %v Received request from: %s, %v %v", time.Now(), in.Name, peer, ok)
	z := pb.ExposedServicesReply{
		Name: "Exposed Services for " + in.Name,
	}
	err := seReconciler.List(ctx, &list)
	if err == nil {
		for _, v := range list.Items {
			entry := pb.ExposedServicesReply_ExposedService{
				Name: v.Spec.Name,
			}
			for _, w := range v.Spec.Endpoints {
				entry.Endpoints = append(entry.Endpoints, w)
			}
			z.ExposedServices = append(z.ExposedServices, &entry)
		}
	}
	return &z, err
}

// Discovery creates a grpc server
func Discovery(ser *controllers.ServiceExpositionReconciler) {

	if ser == nil {
		log.Fatalf("Need Service Exposition Reconciler; None provided")
	}
	seReconciler = ser

	lis, err := net.Listen("tcp", port)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	s := grpc.NewServer()
	pb.RegisterESDSServer(s, &server{})
	// Register reflection service on gRPC server.
	reflection.Register(s)
	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
