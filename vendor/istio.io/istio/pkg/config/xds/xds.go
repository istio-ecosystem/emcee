// Copyright 2019 Istio Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package xds

import (
	"bytes"
	"errors"
	"fmt"

	xdsAPI "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	listener "github.com/envoyproxy/go-control-plane/envoy/api/v2/listener"
	route "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	httpConn "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/http_connection_manager/v2"
	gogojsonpb "github.com/gogo/protobuf/jsonpb"
	"github.com/gogo/protobuf/types"
	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"

	networking "istio.io/api/networking/v1alpha3"
)

// nolint: interfacer
func BuildXDSObjectFromStruct(applyTo networking.EnvoyFilter_ApplyTo, value *types.Struct) (proto.Message, error) {
	if value == nil {
		// for remove ops
		return nil, nil
	}
	var obj proto.Message
	switch applyTo {
	case networking.EnvoyFilter_CLUSTER:
		obj = &xdsAPI.Cluster{}
	case networking.EnvoyFilter_LISTENER:
		obj = &xdsAPI.Listener{}
	case networking.EnvoyFilter_ROUTE_CONFIGURATION:
		obj = &xdsAPI.RouteConfiguration{}
	case networking.EnvoyFilter_FILTER_CHAIN:
		obj = &listener.FilterChain{}
	case networking.EnvoyFilter_HTTP_FILTER:
		obj = &httpConn.HttpFilter{}
	case networking.EnvoyFilter_NETWORK_FILTER:
		obj = &listener.Filter{}
	case networking.EnvoyFilter_VIRTUAL_HOST:
		obj = &route.VirtualHost{}
	case networking.EnvoyFilter_HTTP_ROUTE:
		obj = &route.Route{}
	default:
		return nil, fmt.Errorf("Envoy filter: unknown object type for applyTo %s", applyTo.String()) // nolint: golint,stylecheck
	}

	if err := GogoStructToMessage(value, obj); err != nil {
		return nil, fmt.Errorf("Envoy filter: %v", err) // nolint: golint,stylecheck
	}
	return obj, nil
}

func GogoStructToMessage(pbst *types.Struct, out proto.Message) error {
	if pbst == nil {
		return errors.New("nil struct")
	}

	buf := &bytes.Buffer{}
	if err := (&gogojsonpb.Marshaler{OrigName: true}).Marshal(buf, pbst); err != nil {
		return err
	}

	return jsonpb.Unmarshal(buf, out)
}
