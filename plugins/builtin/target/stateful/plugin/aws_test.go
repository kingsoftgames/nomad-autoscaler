package plugin

import (
	"errors"
	"testing"
	"time"

	"github.com/hashicorp/nomad-autoscaler/plugins/builtin/target/stateful/utils"
	"github.com/stretchr/testify/assert"
)

func TestTargetPlugin_generateScaleReq(t *testing.T) {
	testCases := []struct {
		inputNum            int64
		inputConfig         map[string]string
		expectedOutputReq   *utils.ScaleInReq
		expectedOutputError error
		name                string
	}{
		{
			inputNum: 2,
			inputConfig: map[string]string{
				"node_class":          "high-memory",
				"node_drain_deadline": "5m",
			},
			expectedOutputReq: &utils.ScaleInReq{
				Num:           2,
				DrainDeadline: 5 * time.Minute,
				PoolIdentifier: &utils.PoolIdentifier{
					IdentifierKey: utils.IdentifierKeyClass,
					Value:         "high-memory",
				},
				RemoteProvider: utils.RemoteProviderAWSInstanceID,
				NodeIDStrategy: utils.IDStrategyNewestCreateIndex,
			},
			expectedOutputError: nil,
			name:                "valid request with drain_deadline in config",
		},
		{
			inputNum:            2,
			inputConfig:         map[string]string{},
			expectedOutputReq:   nil,
			expectedOutputError: errors.New("required config param \"node_class\" not found"),
			name:                "no class key found in config",
		},
		{
			inputNum: 2,
			inputConfig: map[string]string{
				"node_class": "high-memory",
			},
			expectedOutputReq: &utils.ScaleInReq{
				Num:           2,
				DrainDeadline: 15 * time.Minute,
				PoolIdentifier: &utils.PoolIdentifier{
					IdentifierKey: utils.IdentifierKeyClass,
					Value:         "high-memory",
				},
				RemoteProvider: utils.RemoteProviderAWSInstanceID,
				NodeIDStrategy: utils.IDStrategyNewestCreateIndex,
			},
			expectedOutputError: nil,
			name:                "drain_deadline not specified within config",
		},
		{
			inputNum: 2,
			inputConfig: map[string]string{
				"node_class":          "high-memory",
				"node_drain_deadline": "time to make a cuppa",
			},
			expectedOutputReq:   nil,
			expectedOutputError: errors.New("failed to parse \"time to make a cuppa\" as time duration"),
			name:                "malformed drain_deadline config value",
		},
	}

	tp := TargetPlugin{}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actualReq, actualErr := tp.generateScaleReq(tc.inputNum, tc.inputConfig)
			assert.Equal(t, tc.expectedOutputReq, actualReq, tc.name)
			assert.Equal(t, tc.expectedOutputError, actualErr, tc.name)
		})
	}
}
