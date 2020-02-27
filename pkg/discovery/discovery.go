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
	"io"
	"net"
	"sync"

	mmv1 "github.com/istio-ecosystem/emcee/api/v1"
	"github.com/istio-ecosystem/emcee/controllers"
	pb "github.com/istio-ecosystem/emcee/pkg/discovery/api"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"
	"istio.io/pkg/log"
)

var (
	seReconciler *controllers.ServiceExpositionReconciler

	esdsClients      = map[string]*EsdsConnection{}
	esdsClientsMutex sync.RWMutex
)

// server is used to implement Exposed Services Discovery Service.
type server struct {
	grpc.ServerStream
}

// EsdsEvent represents a config or registry event that results in a push.
type EsdsEvent struct {
	// function to call once a push is finished. This must be called or future changes may be blocked.
	done func()
}

type EsdsConnection struct {
	// PeerAddr is the address of the client envoy, from network layer
	PeerAddr string

	// ConID is the connection identifier, used as a key in the connection table.
	// Currently based on the node name and a counter.
	ConID string

	// Both ADS and EDS streams implement this interface
	stream pb.ESDS_ExposedServicesDiscoveryServer

	// Sending on this channel results in a push. We may also make it a channel of objects so
	// same info can be sent to all clients, without recomputing.
	pushChannel chan *EsdsEvent

	mutex sync.RWMutex
	added bool
}

func getAllExposedService(z, in *pb.ExposedServicesMessages) {
	var list mmv1.ServiceExpositionList
	err := seReconciler.List(context.Background(), &list)
	z.Name = "Exposed Services for " + in.Name

	if err == nil {
		for _, v := range list.Items {
			name := v.Spec.Name
			if v.Spec.Alias != "" {
				name = v.Spec.Alias
			}
			entry := pb.ExposedServicesMessages_ExposedService{
				Name:                  name,
				Port:                  v.Spec.Port,
				MeshFedConfigSelector: v.Spec.MeshFedConfigSelector,
			}
			for _, w := range v.Spec.Endpoints {
				entry.Endpoints = append(entry.Endpoints, w)
			}
			z.ExposedServices = append(z.ExposedServices, &entry)
		}
	}
}

func receiveThread(stream pb.ESDS_ExposedServicesDiscoveryServer, reqChannel chan *pb.ExposedServicesMessages, receiveError *error) {
	defer close(reqChannel)
	for {
		req, err := stream.Recv()
		if err != nil {
			if status.Code(err) == codes.Canceled || err == io.EOF {
				log.Warnf("ESDS: terminated %v", err)
				return
			}
			log.Warnf("ESDS: terminated with error: %v", err)
			return
		}
		select {
		case reqChannel <- req:
		case <-stream.Context().Done():
			log.Warnf("ESDS: terminated with stream closed")
			return
		}
	}
}

func updateThread(updateChannel chan int, updateError *error) {
	for {
		select {
		case <-updateChannel:
			for _, v := range esdsClients {
				v.pushChannel <- &EsdsEvent{}
			}
		}
	}
}

// ExposedServicesDiscovery implements ESDS server
func (s *server) ExposedServicesDiscovery(stream pb.ESDS_ExposedServicesDiscoveryServer) error {

	peerInfo, ok := peer.FromContext(stream.Context())
	peerAddr := "0.0.0.0"
	if ok {
		peerAddr = peerInfo.Addr.String()
	}
	con := newEsdsConnection(peerAddr, stream)

	var receiveError error
	reqChannel := make(chan *pb.ExposedServicesMessages)
	go receiveThread(con.stream, reqChannel, &receiveError)

	for {
		// Block until either a request is received or a push is triggered.
		select {
		case discReq, ok := <-reqChannel:
			log.Infof("Received a new REQUEST")
			if !ok {
				// Remote side closed connection.
				return receiveError
			}
			var out pb.ExposedServicesMessages
			getAllExposedService(&out, discReq)
			if err := stream.Send(&out); err != nil {
				return err
			}

			// add connection to list of all connections if not already addes
			con.mutex.Lock()
			if !con.added {
				con.added = true
				addCon(con.ConID, con)
				con.mutex.Unlock()
				defer removeCon(con.ConID, con)
			} else {
				con.mutex.Unlock()
			}
		case <-con.pushChannel:
			log.Infof("Received a new UPDATE")
			in := pb.ExposedServicesMessages{
				Name: "Eventer",
			}
			var out pb.ExposedServicesMessages
			getAllExposedService(&out, &in)
			err := stream.Send(&out)
			//pushEv.done()
			if err != nil {
				log.Fatalf("Discovery Server failed.")
				return nil
			}
		}
	}
}

// Discovery creates a grpc server
func Discovery(ser *controllers.ServiceExpositionReconciler, grpcServerAddr *string) {
	var updateError error
	if ser == nil {
		log.Fatalf("Need Service Exposition Reconciler; None provided")
	}
	seReconciler = ser
	controllers.UpdateChannel = make(chan int)
	go updateThread(controllers.UpdateChannel, &updateError)

	lis, err := net.Listen("tcp", *grpcServerAddr)
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

func newEsdsConnection(peerAddr string, stream pb.ESDS_ExposedServicesDiscoveryServer) *EsdsConnection {
	return &EsdsConnection{
		pushChannel: make(chan *EsdsEvent),
		PeerAddr:    peerAddr,
		ConID:       peerAddr, // TODO: maybe update
		stream:      stream,
	}
}

func addCon(conID string, con *EsdsConnection) {
	esdsClientsMutex.Lock()
	defer esdsClientsMutex.Unlock()
	esdsClients[conID] = con
}

func removeCon(conID string, con *EsdsConnection) {
	esdsClientsMutex.Lock()
	defer esdsClientsMutex.Unlock()

	if _, exist := esdsClients[conID]; !exist {
		log.Warnf("ADS: Removing connection for non-existing node:%v.", conID)
	} else {
		delete(esdsClients, conID)
	}
}
