// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of Cilium

package bgpv2

import (
	"context"
	"maps"
	"testing"

	"github.com/cilium/hive/hivetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/utils/ptr"

	cilium_api_v2 "github.com/cilium/cilium/pkg/k8s/apis/cilium.io/v2"
	cilium_api_v2alpha1 "github.com/cilium/cilium/pkg/k8s/apis/cilium.io/v2alpha1"
	slim_meta_v1 "github.com/cilium/cilium/pkg/k8s/slim/k8s/apis/meta/v1"
	"github.com/cilium/cilium/pkg/time"
)

var (
	cluster1 = cilium_api_v2alpha1.CiliumBGPInstance{
		Name:     "cluster-1-instance-65001",
		LocalASN: ptr.To[int64](65001),
		Peers: []cilium_api_v2alpha1.CiliumBGPPeer{
			{
				Name:        "cluster-1-instance-65002-peer-10.0.0.2",
				PeerAddress: ptr.To[string]("10.0.0.2"),
				PeerASN:     ptr.To[int64](65002),
				PeerConfigRef: &cilium_api_v2alpha1.PeerConfigReference{
					Name: "peer-1",
				},
			},
		},
	}

	expectedNode1 = cilium_api_v2alpha1.CiliumBGPNodeInstance{
		Name:     "cluster-1-instance-65001",
		LocalASN: ptr.To[int64](65001),
		Peers: []cilium_api_v2alpha1.CiliumBGPNodePeer{
			{
				Name:        "cluster-1-instance-65002-peer-10.0.0.2",
				PeerAddress: ptr.To[string]("10.0.0.2"),
				PeerASN:     ptr.To[int64](65002),
				PeerConfigRef: &cilium_api_v2alpha1.PeerConfigReference{
					Name: "peer-1",
				},
			},
		},
	}

	nodeOverride1 = cilium_api_v2alpha1.CiliumBGPNodeConfigOverrideSpec{
		BGPInstances: []cilium_api_v2alpha1.CiliumBGPNodeConfigInstanceOverride{
			{
				Name:      "cluster-1-instance-65001",
				RouterID:  ptr.To[string]("10.10.10.10"),
				LocalPort: ptr.To[int32](5400),
				Peers: []cilium_api_v2alpha1.CiliumBGPNodeConfigPeerOverride{
					{
						Name:         "cluster-1-instance-65002-peer-10.0.0.2",
						LocalAddress: ptr.To[string]("10.10.10.1"),
					},
				},
			},
		},
	}

	expectedNodeWithOverride1 = cilium_api_v2alpha1.CiliumBGPNodeInstance{
		Name:      "cluster-1-instance-65001",
		LocalASN:  ptr.To[int64](65001),
		RouterID:  ptr.To[string]("10.10.10.10"),
		LocalPort: ptr.To[int32](5400),
		Peers: []cilium_api_v2alpha1.CiliumBGPNodePeer{
			{
				Name:         "cluster-1-instance-65002-peer-10.0.0.2",
				PeerAddress:  ptr.To[string]("10.0.0.2"),
				PeerASN:      ptr.To[int64](65002),
				LocalAddress: ptr.To[string]("10.10.10.1"),
				PeerConfigRef: &cilium_api_v2alpha1.PeerConfigReference{
					Name: "peer-1",
				},
			},
		},
	}
)

func Test_NodeLabels(t *testing.T) {
	tests := []struct {
		description        string
		node               *cilium_api_v2.CiliumNode
		clusterConfig      *cilium_api_v2alpha1.CiliumBGPClusterConfig
		expectedNodeConfig *cilium_api_v2alpha1.CiliumBGPNodeConfig
	}{
		{
			description: "node without any labels",
			node: &cilium_api_v2.CiliumNode{
				ObjectMeta: meta_v1.ObjectMeta{
					Name: "node-1",
				},
			},
			clusterConfig: &cilium_api_v2alpha1.CiliumBGPClusterConfig{
				ObjectMeta: meta_v1.ObjectMeta{
					Name: "bgp-cluster-config",
				},
				Spec: cilium_api_v2alpha1.CiliumBGPClusterConfigSpec{
					NodeSelector: &slim_meta_v1.LabelSelector{
						MatchLabels: map[string]string{
							"bgp": "rack1",
						},
					},
					BGPInstances: []cilium_api_v2alpha1.CiliumBGPInstance{
						cluster1,
					},
				},
			},
			expectedNodeConfig: nil,
		},
		{
			description: "node with label and cluster config with MatchLabels",
			node: &cilium_api_v2.CiliumNode{
				ObjectMeta: meta_v1.ObjectMeta{
					Name: "node-1",
					Labels: map[string]string{
						"bgp": "rack1",
					},
				},
			},
			clusterConfig: &cilium_api_v2alpha1.CiliumBGPClusterConfig{
				ObjectMeta: meta_v1.ObjectMeta{
					Name: "bgp-cluster-config",
				},
				Spec: cilium_api_v2alpha1.CiliumBGPClusterConfigSpec{
					NodeSelector: &slim_meta_v1.LabelSelector{
						MatchLabels: map[string]string{
							"bgp": "rack1",
						},
					},
					BGPInstances: []cilium_api_v2alpha1.CiliumBGPInstance{
						cluster1,
					},
				},
			},
			expectedNodeConfig: &cilium_api_v2alpha1.CiliumBGPNodeConfig{
				ObjectMeta: meta_v1.ObjectMeta{
					Name: "node-1",
					OwnerReferences: []meta_v1.OwnerReference{
						{
							Kind: cilium_api_v2alpha1.BGPCCKindDefinition,
							Name: "bgp-cluster-config",
						},
					},
				},
				Spec: cilium_api_v2alpha1.CiliumBGPNodeSpec{
					BGPInstances: []cilium_api_v2alpha1.CiliumBGPNodeInstance{
						expectedNode1,
					},
				},
			},
		},
		{
			description: "node with label and cluster config with MatchExpression",
			node: &cilium_api_v2.CiliumNode{
				ObjectMeta: meta_v1.ObjectMeta{
					Name: "node-1",
					Labels: map[string]string{
						"bgp": "rack1",
					},
				},
			},
			clusterConfig: &cilium_api_v2alpha1.CiliumBGPClusterConfig{
				ObjectMeta: meta_v1.ObjectMeta{
					Name: "bgp-cluster-config",
				},
				Spec: cilium_api_v2alpha1.CiliumBGPClusterConfigSpec{
					NodeSelector: &slim_meta_v1.LabelSelector{
						MatchExpressions: []slim_meta_v1.LabelSelectorRequirement{
							{
								Key:      "bgp",
								Operator: slim_meta_v1.LabelSelectorOpIn,
								Values:   []string{"rack1"},
							},
						},
					},
					BGPInstances: []cilium_api_v2alpha1.CiliumBGPInstance{
						cluster1,
					},
				},
			},
			expectedNodeConfig: &cilium_api_v2alpha1.CiliumBGPNodeConfig{
				ObjectMeta: meta_v1.ObjectMeta{
					Name: "node-1",
					OwnerReferences: []meta_v1.OwnerReference{
						{
							Kind: cilium_api_v2alpha1.BGPCCKindDefinition,
							Name: "bgp-cluster-config",
						},
					},
				},
				Spec: cilium_api_v2alpha1.CiliumBGPNodeSpec{
					BGPInstances: []cilium_api_v2alpha1.CiliumBGPNodeInstance{
						expectedNode1,
					},
				},
			},
		},
		{
			description: "node with label and cluster config with nil node selector",
			node: &cilium_api_v2.CiliumNode{
				ObjectMeta: meta_v1.ObjectMeta{
					Name: "node-1",
					Labels: map[string]string{
						"bgp": "rack1",
					},
				},
			},
			clusterConfig: &cilium_api_v2alpha1.CiliumBGPClusterConfig{
				ObjectMeta: meta_v1.ObjectMeta{
					Name: "bgp-cluster-config",
				},
				Spec: cilium_api_v2alpha1.CiliumBGPClusterConfigSpec{
					NodeSelector: nil,
					BGPInstances: []cilium_api_v2alpha1.CiliumBGPInstance{
						cluster1,
					},
				},
			},
			expectedNodeConfig: &cilium_api_v2alpha1.CiliumBGPNodeConfig{
				ObjectMeta: meta_v1.ObjectMeta{
					Name: "node-1",
					OwnerReferences: []meta_v1.OwnerReference{
						{
							Kind: cilium_api_v2alpha1.BGPCCKindDefinition,
							Name: "bgp-cluster-config",
						},
					},
				},
				Spec: cilium_api_v2alpha1.CiliumBGPNodeSpec{
					BGPInstances: []cilium_api_v2alpha1.CiliumBGPNodeInstance{
						expectedNode1,
					},
				},
			},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), TestTimeout)
	defer cancel()

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			req := require.New(t)

			f, watcherReady := newFixture(ctx, req)

			tlog := hivetest.Logger(t)
			f.hive.Start(tlog, ctx)
			defer f.hive.Stop(tlog, ctx)

			// blocking till all watchers are ready
			watcherReady()

			// setup node
			upsertNode(req, ctx, f, tt.node)

			// upsert BGP cluster config
			upsertBGPCC(req, ctx, f, tt.clusterConfig)

			// validate node configs
			assert.EventuallyWithT(t, func(c *assert.CollectT) {
				nodeConfigs, err := f.bgpnClient.List(ctx, meta_v1.ListOptions{})
				if err != nil {
					assert.NoError(c, err)
					return
				}

				if tt.expectedNodeConfig == nil {
					assert.Empty(c, nodeConfigs.Items)
					return
				}

				nodeConfig, err := f.bgpnClient.Get(ctx, tt.expectedNodeConfig.Name, meta_v1.GetOptions{})
				if err != nil {
					assert.NoError(c, err)
					return
				}

				assert.True(c, isSameOwner(tt.expectedNodeConfig.GetOwnerReferences(), nodeConfig.GetOwnerReferences()))
				assert.Equal(c, tt.expectedNodeConfig.Spec, nodeConfig.Spec)

			}, TestTimeout, 50*time.Millisecond)
		})
	}
}

// Test_ClusterConfigSteps is step based test to validate the BGP node config controller
func Test_ClusterConfigSteps(t *testing.T) {
	clusterConfigName := "bgp-cluster-config"

	steps := []struct {
		description            string
		clusterConfig          *cilium_api_v2alpha1.CiliumBGPClusterConfig
		nodes                  []*cilium_api_v2.CiliumNode
		nodeOverrides          []*cilium_api_v2alpha1.CiliumBGPNodeConfigOverride
		expectedNodeConfigs    []*cilium_api_v2alpha1.CiliumBGPNodeConfig
		expectedTrueConditions []string
	}{
		{
			description:   "initial node setup",
			clusterConfig: nil,
			nodes: []*cilium_api_v2.CiliumNode{
				{
					ObjectMeta: meta_v1.ObjectMeta{
						Name: "node-1",
						Labels: map[string]string{
							"bgp": "rack1",
						},
					},
				},
				{
					ObjectMeta: meta_v1.ObjectMeta{
						Name: "node-2",
						Labels: map[string]string{
							"bgp": "rack1",
						},
					},
				},
			},
			nodeOverrides:       nil,
			expectedNodeConfigs: nil,
		},
		{
			description: "initial bgp cluster config",
			clusterConfig: &cilium_api_v2alpha1.CiliumBGPClusterConfig{
				ObjectMeta: meta_v1.ObjectMeta{
					Name: clusterConfigName,
				},
				Spec: cilium_api_v2alpha1.CiliumBGPClusterConfigSpec{
					NodeSelector: &slim_meta_v1.LabelSelector{
						MatchLabels: map[string]string{
							"bgp": "rack1",
						},
					},
					BGPInstances: []cilium_api_v2alpha1.CiliumBGPInstance{
						cluster1,
					},
				},
			},
			nodes:         nil,
			nodeOverrides: nil,
			expectedNodeConfigs: []*cilium_api_v2alpha1.CiliumBGPNodeConfig{
				{
					ObjectMeta: meta_v1.ObjectMeta{
						Name: "node-1",
						OwnerReferences: []meta_v1.OwnerReference{
							{
								Name: "bgp-cluster-config",
							},
						},
					},
					Spec: cilium_api_v2alpha1.CiliumBGPNodeSpec{
						BGPInstances: []cilium_api_v2alpha1.CiliumBGPNodeInstance{
							expectedNode1,
						},
					},
				},
				{
					ObjectMeta: meta_v1.ObjectMeta{
						Name: "node-2",
						OwnerReferences: []meta_v1.OwnerReference{
							{
								Name: "bgp-cluster-config",
							},
						},
					},
					Spec: cilium_api_v2alpha1.CiliumBGPNodeSpec{
						BGPInstances: []cilium_api_v2alpha1.CiliumBGPNodeInstance{
							expectedNode1,
						},
					},
				},
			},
		},
		{
			description:   "add node override",
			clusterConfig: nil,
			nodes:         nil,
			nodeOverrides: []*cilium_api_v2alpha1.CiliumBGPNodeConfigOverride{
				{
					ObjectMeta: meta_v1.ObjectMeta{
						Name: "node-1",
					},

					Spec: nodeOverride1,
				},
			},
			expectedNodeConfigs: []*cilium_api_v2alpha1.CiliumBGPNodeConfig{
				{
					ObjectMeta: meta_v1.ObjectMeta{
						Name: "node-1",
						OwnerReferences: []meta_v1.OwnerReference{
							{
								Name: "bgp-cluster-config",
							},
						},
					},
					Spec: cilium_api_v2alpha1.CiliumBGPNodeSpec{
						BGPInstances: []cilium_api_v2alpha1.CiliumBGPNodeInstance{
							expectedNodeWithOverride1,
						},
					},
				},
				{
					ObjectMeta: meta_v1.ObjectMeta{
						Name: "node-2",
						OwnerReferences: []meta_v1.OwnerReference{
							{
								Name: "bgp-cluster-config",
							},
						},
					},
					Spec: cilium_api_v2alpha1.CiliumBGPNodeSpec{
						BGPInstances: []cilium_api_v2alpha1.CiliumBGPNodeInstance{
							expectedNode1,
						},
					},
				},
			},
		},
		{
			description:   "add new node",
			clusterConfig: nil,
			nodes: []*cilium_api_v2.CiliumNode{
				{
					ObjectMeta: meta_v1.ObjectMeta{
						Name: "node-3",
						Labels: map[string]string{
							"bgp": "rack1",
						},
					},
				},
			},
			nodeOverrides: nil,
			expectedNodeConfigs: []*cilium_api_v2alpha1.CiliumBGPNodeConfig{
				{
					ObjectMeta: meta_v1.ObjectMeta{
						Name: "node-1",
						OwnerReferences: []meta_v1.OwnerReference{
							{
								Name: "bgp-cluster-config",
							},
						},
					},
					Spec: cilium_api_v2alpha1.CiliumBGPNodeSpec{
						BGPInstances: []cilium_api_v2alpha1.CiliumBGPNodeInstance{
							expectedNodeWithOverride1,
						},
					},
				},
				{
					ObjectMeta: meta_v1.ObjectMeta{
						Name: "node-2",
						OwnerReferences: []meta_v1.OwnerReference{
							{
								Name: "bgp-cluster-config",
							},
						},
					},
					Spec: cilium_api_v2alpha1.CiliumBGPNodeSpec{
						BGPInstances: []cilium_api_v2alpha1.CiliumBGPNodeInstance{
							expectedNode1,
						},
					},
				},
				{
					ObjectMeta: meta_v1.ObjectMeta{
						Name: "node-3",
						OwnerReferences: []meta_v1.OwnerReference{
							{
								Name: "bgp-cluster-config",
							},
						},
					},
					Spec: cilium_api_v2alpha1.CiliumBGPNodeSpec{
						BGPInstances: []cilium_api_v2alpha1.CiliumBGPNodeInstance{
							expectedNode1,
						},
					},
				},
			},
		},
		{
			description:   "remove node labels",
			clusterConfig: nil,
			nodes: []*cilium_api_v2.CiliumNode{
				{
					ObjectMeta: meta_v1.ObjectMeta{
						Name: "node-1",
					},
				},
				{
					ObjectMeta: meta_v1.ObjectMeta{
						Name: "node-2",
					},
				},
				{
					ObjectMeta: meta_v1.ObjectMeta{
						Name: "node-3",
					},
				},
			},
			nodeOverrides:       nil,
			expectedNodeConfigs: nil,
			expectedTrueConditions: []string{
				cilium_api_v2alpha1.BGPClusterConfigConditionNoMatchingNode,
			},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), TestTimeout)
	defer cancel()

	f, watchersReady := newFixture(ctx, require.New(t))

	tlog := hivetest.Logger(t)
	f.hive.Start(tlog, ctx)
	defer f.hive.Stop(tlog, ctx)

	watchersReady()

	for _, step := range steps {
		t.Run(step.description, func(t *testing.T) {
			req := require.New(t)

			// setup nodes
			for _, node := range step.nodes {
				upsertNode(req, ctx, f, node)
			}

			// upsert BGP cluster config
			upsertBGPCC(req, ctx, f, step.clusterConfig)

			// upsert node overrides
			for _, nodeOverride := range step.nodeOverrides {
				upsertNodeOverrides(req, ctx, f, nodeOverride)
			}

			// validate node configs
			assert.EventuallyWithT(t, func(c *assert.CollectT) {
				nodes, err := f.bgpnClient.List(ctx, meta_v1.ListOptions{})
				if err != nil {
					assert.NoError(c, err)
					return
				}

				assert.Equal(c, len(step.expectedNodeConfigs), len(nodes.Items))

				for _, expectedNodeConfig := range step.expectedNodeConfigs {
					nodeConfig, err := f.bgpnClient.Get(ctx, expectedNodeConfig.Name, meta_v1.GetOptions{})
					if err != nil {
						assert.NoError(c, err)
						return
					}

					assert.Equal(c, expectedNodeConfig.Name, nodeConfig.Name)
					assert.Equal(c, expectedNodeConfig.Spec, nodeConfig.Spec)
				}
			}, TestTimeout, 50*time.Millisecond)

			// Condition checks. Assuming the cluster config already exists on the API server.
			if len(step.expectedTrueConditions) > 0 {
				bgpcc, err := f.bgpcClient.Get(ctx, clusterConfigName, meta_v1.GetOptions{})
				req.NoError(err)

				trueConditions := sets.New[string]()
				for _, cond := range bgpcc.Status.Conditions {
					trueConditions.Insert(cond.Type)
				}

				for _, cond := range step.expectedTrueConditions {
					req.True(trueConditions.Has(cond), "Condition missing or not true: %s", cond)
				}
			}
		})
	}
}

func TestClusterConfigConditions(t *testing.T) {
	clusterConfigName := "cluster-config0"
	peerConfigName := "peer-config0"

	node := cilium_api_v2.CiliumNode{
		ObjectMeta: meta_v1.ObjectMeta{
			Name: "node-1",
			Labels: map[string]string{
				"bgp": "rack1",
			},
		},
	}

	peerConfig := cilium_api_v2alpha1.CiliumBGPPeerConfig{
		ObjectMeta: meta_v1.ObjectMeta{
			Name: peerConfigName,
		},
	}

	tests := []struct {
		name                    string
		clusterConfig           *cilium_api_v2alpha1.CiliumBGPClusterConfig
		expectedConditionStatus map[string]meta_v1.ConditionStatus
	}{
		{
			name: "NoMatchingNode False",
			clusterConfig: &cilium_api_v2alpha1.CiliumBGPClusterConfig{
				ObjectMeta: meta_v1.ObjectMeta{
					Name: clusterConfigName,
				},
				Spec: cilium_api_v2alpha1.CiliumBGPClusterConfigSpec{
					NodeSelector: &slim_meta_v1.LabelSelector{
						MatchLabels: map[string]string{
							"bgp": "rack1",
						},
					},
				},
			},
			expectedConditionStatus: map[string]meta_v1.ConditionStatus{
				cilium_api_v2alpha1.BGPClusterConfigConditionNoMatchingNode: meta_v1.ConditionFalse,
			},
		},
		{
			name: "NoMatchingNode False Nil Selector",
			clusterConfig: &cilium_api_v2alpha1.CiliumBGPClusterConfig{
				ObjectMeta: meta_v1.ObjectMeta{
					Name: clusterConfigName,
				},
				Spec: cilium_api_v2alpha1.CiliumBGPClusterConfigSpec{
					NodeSelector: nil,
				},
			},
			expectedConditionStatus: map[string]meta_v1.ConditionStatus{
				cilium_api_v2alpha1.BGPClusterConfigConditionNoMatchingNode: meta_v1.ConditionFalse,
			},
		},
		{
			name: "NoMatchingNode True",
			clusterConfig: &cilium_api_v2alpha1.CiliumBGPClusterConfig{
				ObjectMeta: meta_v1.ObjectMeta{
					Name: clusterConfigName,
				},
				Spec: cilium_api_v2alpha1.CiliumBGPClusterConfigSpec{
					NodeSelector: &slim_meta_v1.LabelSelector{
						MatchLabels: map[string]string{
							"bgp": "rack2",
						},
					},
				},
			},
			expectedConditionStatus: map[string]meta_v1.ConditionStatus{
				cilium_api_v2alpha1.BGPClusterConfigConditionNoMatchingNode: meta_v1.ConditionTrue,
			},
		},
		{
			name: "MissingPeerConfig False",
			clusterConfig: &cilium_api_v2alpha1.CiliumBGPClusterConfig{
				ObjectMeta: meta_v1.ObjectMeta{
					Name: clusterConfigName,
				},
				Spec: cilium_api_v2alpha1.CiliumBGPClusterConfigSpec{
					NodeSelector: nil,
					BGPInstances: []cilium_api_v2alpha1.CiliumBGPInstance{
						{
							Peers: []cilium_api_v2alpha1.CiliumBGPPeer{
								{
									Name: "peer0",
									PeerConfigRef: &cilium_api_v2alpha1.PeerConfigReference{
										Name: peerConfigName,
									},
								},
							},
						},
					},
				},
			},
			expectedConditionStatus: map[string]meta_v1.ConditionStatus{
				cilium_api_v2alpha1.BGPClusterConfigConditionMissingPeerConfigs: meta_v1.ConditionFalse,
			},
		},
		{
			name: "MissingPeerConfig False nil PeerConfigRef",
			clusterConfig: &cilium_api_v2alpha1.CiliumBGPClusterConfig{
				ObjectMeta: meta_v1.ObjectMeta{
					Name: clusterConfigName,
				},
				Spec: cilium_api_v2alpha1.CiliumBGPClusterConfigSpec{
					NodeSelector: nil,
					BGPInstances: []cilium_api_v2alpha1.CiliumBGPInstance{
						{
							Peers: []cilium_api_v2alpha1.CiliumBGPPeer{
								{
									Name: "peer0",
								},
							},
						},
					},
				},
			},
			expectedConditionStatus: map[string]meta_v1.ConditionStatus{
				cilium_api_v2alpha1.BGPClusterConfigConditionMissingPeerConfigs: meta_v1.ConditionFalse,
			},
		},
		{
			name: "MissingPeerConfig True",
			clusterConfig: &cilium_api_v2alpha1.CiliumBGPClusterConfig{
				ObjectMeta: meta_v1.ObjectMeta{
					Name: clusterConfigName,
				},
				Spec: cilium_api_v2alpha1.CiliumBGPClusterConfigSpec{
					NodeSelector: nil,
					BGPInstances: []cilium_api_v2alpha1.CiliumBGPInstance{
						{
							Peers: []cilium_api_v2alpha1.CiliumBGPPeer{
								{
									Name: "peer0",
									PeerConfigRef: &cilium_api_v2alpha1.PeerConfigReference{
										Name: peerConfigName + "-foo",
									},
								},
							},
						},
					},
				},
			},
			expectedConditionStatus: map[string]meta_v1.ConditionStatus{
				cilium_api_v2alpha1.BGPClusterConfigConditionMissingPeerConfigs: meta_v1.ConditionTrue,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := require.New(t)

			ctx, cancel := context.WithTimeout(context.Background(), TestTimeout)
			defer cancel()

			f, watchersReady := newFixture(ctx, require.New(t))

			tlog := hivetest.Logger(t)
			f.hive.Start(tlog, ctx)
			defer f.hive.Stop(tlog, ctx)

			watchersReady()

			// Setup resources
			upsertNode(req, ctx, f, &node)
			upsertBGPCC(req, ctx, f, tt.clusterConfig)
			upsertBGPPC(req, ctx, f, &peerConfig)

			require.EventuallyWithT(t, func(ct *assert.CollectT) {
				// Check conditions
				cc, err := f.bgpcClient.Get(ctx, clusterConfigName, meta_v1.GetOptions{})
				if !assert.NoError(ct, err, "Cannot get cluster config") {
					return
				}

				// Check if the expected condition exists and has an intended values
				missing := maps.Clone(tt.expectedConditionStatus)
				for condType, status := range tt.expectedConditionStatus {
					for _, cond := range cc.Status.Conditions {
						if cond.Type == condType {
							if !assert.Equal(ct, status, cond.Status) {
								return
							}
							delete(missing, cond.Type)
						}
					}
				}

				assert.Empty(ct, missing, "Missing conditions: %v", missing)
			}, time.Second*3, time.Millisecond*100)
		})
	}
}

func upsertNode(req *require.Assertions, ctx context.Context, f *fixture, node *cilium_api_v2.CiliumNode) {
	_, err := f.nodeClient.Get(ctx, node.Name, meta_v1.GetOptions{})
	if err != nil && k8sErrors.IsNotFound(err) {
		_, err = f.nodeClient.Create(ctx, node, meta_v1.CreateOptions{})
	} else if err != nil {
		req.Fail(err.Error())
	} else {
		_, err = f.nodeClient.Update(ctx, node, meta_v1.UpdateOptions{})
	}
	req.NoError(err)
}

func upsertBGPCC(req *require.Assertions, ctx context.Context, f *fixture, bgpcc *cilium_api_v2alpha1.CiliumBGPClusterConfig) {
	if bgpcc == nil {
		return
	}

	_, err := f.bgpcClient.Get(ctx, bgpcc.Name, meta_v1.GetOptions{})
	if err != nil && k8sErrors.IsNotFound(err) {
		_, err = f.bgpcClient.Create(ctx, bgpcc, meta_v1.CreateOptions{})
	} else if err != nil {
		req.Fail(err.Error())
	} else {
		_, err = f.bgpcClient.Update(ctx, bgpcc, meta_v1.UpdateOptions{})
	}
	req.NoError(err)
}

func upsertBGPPC(req *require.Assertions, ctx context.Context, f *fixture, bgppc *cilium_api_v2alpha1.CiliumBGPPeerConfig) {
	if bgppc == nil {
		return
	}

	_, err := f.bgpcClient.Get(ctx, bgppc.Name, meta_v1.GetOptions{})
	if err != nil && k8sErrors.IsNotFound(err) {
		_, err = f.bgppcClient.Create(ctx, bgppc, meta_v1.CreateOptions{})
	} else if err != nil {
		req.Fail(err.Error())
	} else {
		_, err = f.bgppcClient.Update(ctx, bgppc, meta_v1.UpdateOptions{})
	}
	req.NoError(err)
}

func upsertNodeOverrides(req *require.Assertions, ctx context.Context, f *fixture, nodeOverride *cilium_api_v2alpha1.CiliumBGPNodeConfigOverride) {
	_, err := f.bgpncoClient.Get(ctx, nodeOverride.Name, meta_v1.GetOptions{})
	if err != nil && k8sErrors.IsNotFound(err) {
		_, err = f.bgpncoClient.Create(ctx, nodeOverride, meta_v1.CreateOptions{})
	} else {
		_, err = f.bgpncoClient.Update(ctx, nodeOverride, meta_v1.UpdateOptions{})
	}
	req.NoError(err)
}

func isSameOwner(expectedOwners, runningOwners []meta_v1.OwnerReference) bool {
	if len(expectedOwners) != len(runningOwners) {
		return false
	}

	for i, owner := range expectedOwners {
		if runningOwners[i].Kind != owner.Kind || runningOwners[i].Name != owner.Name {
			return false
		}
	}
	return true
}
