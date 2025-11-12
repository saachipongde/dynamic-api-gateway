package main

import (
	"context"
	"log"
	"net"
	"strconv"
	"time"

	// Import all the necessary Envoy and Go control plane packages
	cluster "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	endpoint "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	route "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	discovery "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	"github.com/envoyproxy/go-control-plane/pkg/cache/types"
	"github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	"github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	"github.com/envoyproxy/go-control-plane/pkg/server/v3"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/wrapperspb"

	// Prometheus imports
	"github.com/prometheus/client_golang/api"
	prometheusv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	p_model "github.com/prometheus/common/model"
)

const (
	nodeID    = "test-id"
	routeName = "local_route"
	// Prometheus is reachable via its service name from docker-compose
	prometheusURL = "http://prometheus:9090"
)

// makeClusters creates the Cluster definitions for service-a and service-b
// It now accepts weights for load balancing.
func makeClusters(weightA, weightB uint32) []types.Resource {
	// Cluster definition for service-a
	serviceACluster := &cluster.Cluster{
		Name:                 "service_a_cluster",
		ClusterDiscoveryType: &cluster.Cluster_Type{Type: cluster.Cluster_STRICT_DNS},
		// Use WEIGHTED_ROUND_ROBIN for load balancing
		LbPolicy: cluster.Cluster_ROUND_ROBIN,
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
												Address:   "service-a",
												PortSpecifier: &core.SocketAddress_PortValue{
													PortValue: 3000,
												},
											},
										},
									},
								},
							},
							// Assign the weight for this endpoint
							LoadBalancingWeight: wrapperspb.UInt32(weightA),
						},
					},
				},
			},
		},
	}

	// Cluster definition for service-b
	serviceBCluster := &cluster.Cluster{
		Name:                 "service_b_cluster",
		ClusterDiscoveryType: &cluster.Cluster_Type{Type: cluster.Cluster_STRICT_DNS},
		LbPolicy:             cluster.Cluster_ROUND_ROBIN,
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
												Address:   "service-b",
												PortSpecifier: &core.SocketAddress_PortValue{
													PortValue: 3000,
												},
											},
										},
									},
								},
							},
							// Assign the weight for this endpoint
							LoadBalancingWeight: wrapperspb.UInt32(weightB),
						},
					},
				},
			},
		},
	}

	return []types.Resource{serviceACluster, serviceBCluster}
}

// makeRoutes creates the Route configuration.
// This is now DYNAMIC. It will send traffic to *both* clusters
// and let the Cluster's LbPolicy handle the weighted split.
func makeRoutes() []types.Resource {
	routeConfig := &route.RouteConfiguration{
		Name: routeName,
		VirtualHosts: []*route.VirtualHost{
			{
				Name:    "local_service",
				Domains: []string{"*"},
				Routes: []*route.Route{
					// This route handles *all* traffic (for /a and /b)
					// and splits it between the two clusters.
					{
						Match: &route.RouteMatch{
							PathSpecifier: &route.RouteMatch_Prefix{
								Prefix: "/",
							},
						},
						Action: &route.Route_Route{
							Route: &route.RouteAction{
								// Use a WeightedCluster to split traffic
								ClusterSpecifier: &route.RouteAction_WeightedClusters{
									WeightedClusters: &route.WeightedCluster{
										TotalWeight: wrapperspb.UInt32(100),
										Clusters: []*route.WeightedCluster_ClusterWeight{
											{
												Name:   "service_a_cluster",
												Weight: wrapperspb.UInt32(50),
											},
											{
												Name:   "service_b_cluster",
												Weight: wrapperspb.UInt32(50),
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

	return []types.Resource{routeConfig}
}

// makeSnapshot creates a "snapshot" of the configuration
// It now takes a version number
func makeSnapshot(version int, weightA, weightB uint32) *cache.Snapshot {
	log.Printf("Creating snapshot version %d with weights A=%d, B=%d", version, weightA, weightB)
	snapshot, err := cache.NewSnapshot(
		// Version must be a string
		strconv.Itoa(version),
		map[resource.Type][]types.Resource{
			// We MUST provide CDS and RDS in our snapshot.
			// The routing logic is now in makeRoutes, not makeClusters.
			// Let's simplify and put the logic in the Route, not the Cluster.
			// This is a much better design.
			
			// Let's fix this. We will NOT use weighted endpoints.
			// We will use a weighted *route*.
			
			// This is the clusters *definition*. They don't need weights.
			resource.ClusterType:  makeStaticClusters(), 
			resource.RouteType:    makeWeightedRoutes(weightA, weightB),
			resource.ListenerType: {},
			resource.EndpointType: {},
		},
	)
	if err != nil {
		log.Fatalf("Failed to create snapshot: %v", err)
	}
	return snapshot
}

// ---- REVISED HELPERS ----

// makeStaticClusters just defines the *existence* of the clusters
func makeStaticClusters() []types.Resource {
	serviceACluster := &cluster.Cluster{ // Cluster A
		Name: "service_a_cluster",
		ClusterDiscoveryType: &cluster.Cluster_Type{Type: cluster.Cluster_STRICT_DNS},
		LoadAssignment: &endpoint.ClusterLoadAssignment{
			ClusterName: "service_a_cluster",
			Endpoints: []*endpoint.LocalityLbEndpoints{{
				LbEndpoints: []*endpoint.LbEndpoint{{
					HostIdentifier: &endpoint.LbEndpoint_Endpoint{
						Endpoint: &endpoint.Endpoint{Address: &core.Address{
							Address: &core.Address_SocketAddress{SocketAddress: &core.SocketAddress{
								Address:   "service-a",
								PortSpecifier: &core.SocketAddress_PortValue{
									PortValue: 3000,
								},
							}},
						}},
					},
				}},
			}},
		},
	}

	serviceBCluster := &cluster.Cluster{ // Cluster B
		Name: "service_b_cluster",
		ClusterDiscoveryType: &cluster.Cluster_Type{Type: cluster.Cluster_STRICT_DNS},
		LoadAssignment: &endpoint.ClusterLoadAssignment{
			ClusterName: "service_b_cluster",
			Endpoints: []*endpoint.LocalityLbEndpoints{{
				LbEndpoints: []*endpoint.LbEndpoint{{
					HostIdentifier: &endpoint.LbEndpoint_Endpoint{
						Endpoint: &endpoint.Endpoint{Address: &core.Address{
							Address: &core.Address_SocketAddress{SocketAddress: &core.SocketAddress{
								Address:   "service-b",
								PortSpecifier: &core.SocketAddress_PortValue{
									PortValue: 3000,
								},
							}},
						}},
					},
				}},
			}},
		},
	}
	return []types.Resource{serviceACluster, serviceBCluster}
}

// makeWeightedRoutes creates the dynamic route with the weights
func makeWeightedRoutes(weightA, weightB uint32) []types.Resource {
	// Ensure total weight is 100 for simplicity
	totalWeight := weightA + weightB
	if totalWeight == 0 {
		weightA = 50
		weightB = 50
	}
	
	// Normalize to 100
	normalizedWeightA := uint32(float64(weightA) / float64(totalWeight) * 100)
	normalizedWeightB := 100 - normalizedWeightA

	routeConfig := &route.RouteConfiguration{
		Name: routeName,
		VirtualHosts: []*route.VirtualHost{
			{
				Name:    "local_service",
				Domains: []string{"*"},
				Routes: []*route.Route{
					{
						// Match requests to /a and route to service A
						Match: &route.RouteMatch{ PathSpecifier: &route.RouteMatch_Prefix{ Prefix: "/a", }, },
						Action: &route.Route_Route{ Route: &route.RouteAction{
							ClusterSpecifier: &route.RouteAction_Cluster{ Cluster: "service_a_cluster", },
							PrefixRewrite: "/",
						}},
					},
					{
						// Match requests to /b and route to service B
						Match: &route.RouteMatch{ PathSpecifier: &route.RouteMatch_Prefix{ Prefix: "/b", }, },
						Action: &route.Route_Route{ Route: &route.RouteAction{
							ClusterSpecifier: &route.RouteAction_Cluster{ Cluster: "service_b_cluster", },
							PrefixRewrite: "/",
						}},
					},
					{
						// NEW: Match requests to / (root) and split them
						Match: &route.RouteMatch{ PathSpecifier: &route.RouteMatch_Prefix{ Prefix: "/", }, },
						Action: &route.Route_Route{
							Route: &route.RouteAction{
								ClusterSpecifier: &route.RouteAction_WeightedClusters{
									WeightedClusters: &route.WeightedCluster{
										TotalWeight: wrapperspb.UInt32(100),
										Clusters: []*route.WeightedCluster_ClusterWeight{
											{
												Name:   "service_a_cluster",
												Weight: wrapperspb.UInt32(normalizedWeightA),
											},
											{
												Name:   "service_b_cluster",
												Weight: wrapperspb.UInt32(normalizedWeightB),
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
	return []types.Resource{routeConfig}
}


// --- END REVISED HELPERS ---


// runSnapshotManager is the background loop
func runSnapshotManager(snapshotCache cache.SnapshotCache) {
	// 1. Create Prometheus client
	client, err := api.NewClient(api.Config{
		Address: prometheusURL,
	})
	if err != nil {
		log.Fatalf("Failed to create Prometheus client: %v", err)
	}
	promAPI := prometheusv1.NewAPI(client)
	log.Println("Prometheus client created")

	// 2. Start the ticker loop
	ticker := time.NewTicker(5 * time.Second)
	version := 0

	for {
		<-ticker.C
		version++

		// 3. Query Prometheus
		// For simplicity, we'll just query for the total request count for each
		queryA := `sum(rate(envoy_cluster_upstream_rq_total{envoy_cluster_name="service_a_cluster"}[1m]))`
		queryB := `sum(rate(envoy_cluster_upstream_rq_total{envoy_cluster_name="service_b_cluster"}[1m]))`
		
		// Don't error check, just get the values (0 if error)
		rpsA := queryPrometheus(promAPI, queryA)
		rpsB := queryPrometheus(promAPI, queryB)

		log.Printf("Prometheus query: rpsA=%.2f, rpsB=%.2f", rpsA, rpsB)

		// 4. Make a decision
		// This is our "adaptive logic".
		// If B is handling more requests than A, shift weight to A.
		// If A is handling more requests than B, shift weight to B.
		// If they are equal (or 0), keep 50/50.
		
		var weightA, weightB uint32 = 50, 50
		
		if rpsA > rpsB && rpsA > 0 {
			weightA = 30 // A is busier, give it less
			weightB = 70 // B is freer, give it more
		} else if rpsB > rpsA && rpsB > 0 {
			weightA = 70 // A is freer, give it more
			weightB = 30 // B is busier, give it less
		}

		// 5. Create and set the new snapshot
		snapshot := makeSnapshot(version, weightA, weightB)
		if err := snapshotCache.SetSnapshot(context.Background(), nodeID, snapshot); err != nil {
			log.Printf("Failed to set snapshot: %v", err)
		}
	}
}

// queryPrometheus is a helper to run a query and return a float value
func queryPrometheus(api prometheusv1.API, query string) float64 {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	result, warnings, err := api.Query(ctx, query, time.Now())
	if err != nil {
		log.Printf("Prometheus query error: %v", err)
		return 0
	}
	if len(warnings) > 0 {
		log.Printf("Prometheus query warnings: %v", warnings)
	}

	// Result is a vector. We just want the first (and only) value.
	vec, ok := result.(p_model.Vector)
	if !ok || len(vec) == 0 {
		return 0
	}
	return float64(vec[0].Value)
}


// --- MAIN FUNCTION ---

func main() {
	// 1. Create a "cache"
	snapshotCache := cache.NewSnapshotCache(true, cache.IDHash{}, nil)

	// 2. Start the snapshot manager in a new goroutine
	go runSnapshotManager(snapshotCache)

	// 3. Create the gRPC server
	ctx := context.Background()
	srv := server.NewServer(ctx, snapshotCache, nil)

	// 4. Register the Aggregated Discovery Service (ADS)
	grpcServer := grpc.NewServer()
	discovery.RegisterAggregatedDiscoveryServiceServer(grpcServer, srv)

	// 5. Start the gRPC server on port 18000
	lis, err := net.Listen("tcp", ":18000")
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	log.Println("Control plane gRPC server listening on :18000")
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("Failed to start gRPC server: %v", err)
	}
}