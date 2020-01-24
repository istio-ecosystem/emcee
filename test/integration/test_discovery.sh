./emcee --context $CLUSTER1 --metrics-addr ":8081" --grpc-server-addr ":50051" --grpc-discovery-addr "localhost:50052"

./emcee --context $CLUSTER2 --metrics-addr ":8082" --grpc-server-addr ":50052" --grpc-discovery-addr "localhost:50051"