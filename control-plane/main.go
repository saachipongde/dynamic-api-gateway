package main

import (
	"context"
	"log"
	"net"

	// Import all the necessary Envoy and Go control plane packages
	cluster "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	endpoint "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	route "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	discovery "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	"github.com/envoyproxy/go-control-plane/pkg/cache/types" // <<< THIS LINE IS NOW FIXED
	"github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	"github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	"github.com/envoyproxy/go-control-plane/pkg/server/v3"
	"google.golang.org/grpc"
)

const (
	// This is the "node ID" that Envoy will use to identify itself.
	// We're using a simple static one for now.
	nodeID = "test-id"
	// This is the route_config_name from your envoy.yaml
	routeName = "local_route"
)

// makeClusters creates the Cluster definitions for service-a and service-b
func makeClusters() []types.Resource {
	// Cluster definition for service-a
	serviceACluster := &cluster.Cluster{
		Name: "service_a_cluster",
		// Use STRICT_DNS to resolve the service name from docker-compose
		ClusterDiscoveryType: &cluster.Cluster_Type{Type: cluster.Cluster_STRICT_DNS},
		LoadAssignment: &endpoint.ClusterLoadAssignment{
			ClusterName: "service_a_cluster",
			Endpoints: []*endpoint.LocalityLbEndpoints{
				{
					LbEndpoints: []*endpoint.LbEndpoint{
						{
							HostIdentifier: &endpoint.LbEndpoint_Endpoint{
								Endpoint: &endpoint.Endpoint{
									Address: &core.Address{
										Address: &core.Address_SocketAddress{
											SocketAddress: &core.SocketAddress{
												// This is the service name and port from docker-compose
												Address:   "service-a",
												PortSpecifier: &core.SocketAddress_PortValue{
													PortValue: 3000,
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	// Cluster definition for service-b
	serviceBCluster := &cluster.Cluster{
		Name: "service_b_cluster",
		ClusterDiscoveryType: &cluster.Cluster_Type{Type: cluster.Cluster_STRICT_DNS},
		LoadAssignment: &endpoint.ClusterLoadAssignment{
			ClusterName: "service_b_cluster",
			Endpoints: []*endpoint.LocalityLbEndpoints{
				{
					LbEndpoints: []*endpoint.LbEndpoint{
						{
							HostIdentifier: &endpoint.LbEndpoint_Endpoint{
								Endpoint: &endpoint.Endpoint{
									Address: &core.Address{
										Address: &core.Address_SocketAddress{
											SocketAddress: &core.SocketAddress{
												// This is the service name and port from docker-compose
												Address:   "service-b",
												PortSpecifier: &core.SocketAddress_PortValue{
													PortValue: 3000,
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	// We just return a slice of the cluster objects
	return []types.Resource{serviceACluster, serviceBCluster}
}

// makeRoutes creates the Route configuration that Envoy asked for (local_route)
func makeRoutes() []types.Resource {
	// Route configuration
	routeConfig := &route.RouteConfiguration{
		Name: routeName,
		VirtualHosts: []*route.VirtualHost{
			{
				Name:    "local_service",
				Domains: []string{"*"}, // Match all domains
				Routes: []*route.Route{
					// Route for /a
					{
						Match: &route.RouteMatch{
							PathSpecifier: &route.RouteMatch_Prefix{
								Prefix: "/a",
							},
						},
						Action: &route.Route_Route{
							Route: &route.RouteAction{
								// Route to service_a_cluster
								ClusterSpecifier: &route.RouteAction_Cluster{
									Cluster: "service_a_cluster",
								},
								// Rewrite /a to / so the service receives the root request
								PrefixRewrite: "/",
							},
						},
					},
					// Route for /b
					{
						Match: &route.RouteMatch{
							PathSpecifier: &route.RouteMatch_Prefix{
								Prefix: "/b",
							},
						},
						Action: &route.Route_Route{
							Route: &route.RouteAction{
								// Route to service_b_cluster
								ClusterSpecifier: &route.RouteAction_Cluster{
									Cluster: "service_b_cluster",
								},
								PrefixRewrite: "/",
							},
						},
					},
				},
			},
		},
	}

	// We just return a slice containing the single route configuration
	return []types.Resource{routeConfig}
}

// makeSnapshot creates a "snapshot" of the configuration
func makeSnapshot() *cache.Snapshot {
	snapshot, err := cache.NewSnapshot(
		"1", // Version 1
		map[resource.Type][]types.Resource{
			resource.ClusterType: makeClusters(),
			resource.RouteType:   makeRoutes(),
			resource.ListenerType: {}, // <-- ADD THIS LINE
			resource.EndpointType: {}, // <-- ADD THIS LINE
			// We can also send Listener (LDS) and Endpoint (EDS) configs
		},
	)
	if err != nil {
		log.Fatalf("Failed to create snapshot: %v", err)
	}
	return snapshot
}

func main() {
	// 1. Create a "cache"
	// This cache holds the configuration snapshot
	snapshotCache := cache.NewSnapshotCache(true, cache.IDHash{}, nil)

	// 2. Create the snapshot and add it to the cache
	snapshot := makeSnapshot()
	if err := snapshotCache.SetSnapshot(context.Background(), nodeID, snapshot); err != nil {
		log.Fatalf("Failed to set snapshot: %v", err)
	}

	// 3. Create the gRPC server
	ctx := context.Background()
	
	srv := server.NewServer(ctx, snapshotCache, nil)

	// 4. Register the Aggregated Discovery Service (ADS)
	grpcServer := grpc.NewServer()

	// This is the line that caused the error. It's now fixed
	// by moving the import to the top.
	discovery.RegisterAggregatedDiscoveryServiceServer(grpcServer, srv)

	// 5. Start the gRPC server on port 18000 (without TLS)
	lis, err := net.Listen("tcp", ":18000")
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	log.Println("Control plane gRPC server listening on :18000")
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("Failed to start gRPC server: %v", err)
	}
}