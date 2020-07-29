package utils

import (
	"strconv"
	"strings"
)

// Nodes is used to query node-related API endpoints
type Dms struct {
	client *DmsApiClient
}
type DmsNodes struct {
	Nodes map[string]bool
}

// List is used to list out all of the nodes
func (n *Dms) List() (*DmsNodes, error) {
	var resp DmsNodes
	err := n.client.query("/v1/nodes", &resp)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

// Nodes returns a handle on the node endpoints.
func (c *DmsApiClient) Dms() *Dms {
	return &Dms{client: c}
}

const (
	configKeyDmsAddress       = "dms_address"
	configKeyDmsToken         = "dms_token"
	configKeyDmsHTTPAuth      = "dms_http-auth"
	configKeyDmsCACert        = "dms_ca-cert"
	configKeyDmsCAPath        = "dms_ca-path"
	configKeyDmsClientCert    = "dms_client-cert"
	configKeyDmsClientKey     = "dms_client-key"
	configKeyDmsTLSServerName = "dms_tls-server-name"
	configKeyDmsSkipVerify    = "dms_skip-verify"
)

// ConfigFromNamespacedMap converts the map representation of a Dms config to
// the proper object that can be used to setup a client.
func DmsConfigFromMap(cfg map[string]string) *DmsApiConfig {
	c := &DmsApiConfig{
		TLSConfig: &TLSConfig{},
	}

	if addr, ok := cfg[configKeyDmsAddress]; ok {
		c.Address = addr
	}

	if token, ok := cfg[configKeyDmsToken]; ok {
		c.SecretID = token
	}
	if caCert, ok := cfg[configKeyDmsCACert]; ok {
		c.TLSConfig.CACert = caCert
	}
	if caPath, ok := cfg[configKeyDmsCAPath]; ok {
		c.TLSConfig.CAPath = caPath
	}
	if clientCert, ok := cfg[configKeyDmsClientCert]; ok {
		c.TLSConfig.ClientCert = clientCert
	}
	if clientKey, ok := cfg[configKeyDmsClientKey]; ok {
		c.TLSConfig.ClientKey = clientKey
	}
	if serverName, ok := cfg[configKeyDmsTLSServerName]; ok {
		c.TLSConfig.TLSServerName = serverName
	}
	// It should be safe to ignore any error when converting the string to a
	// bool. The boolean value should only ever come from a bool-flag, and
	// therefore we shouldn't have any risk of incorrect or malformed user
	// input string data.
	if skipVerify, ok := cfg[configKeyDmsSkipVerify]; ok {
		c.TLSConfig.Insecure, _ = strconv.ParseBool(skipVerify)
	}
	if httpAuth, ok := cfg[configKeyDmsHTTPAuth]; ok {
		c.HttpAuth = HTTPAuthFromString(httpAuth)
	}

	return c
}

// HTTPAuthFromString take an input string, and converts this to a Nomad API
// representation of basic HTTP auth.
func HTTPAuthFromString(auth string) *HttpBasicAuth {
	if auth == "" {
		return nil
	}

	var username, password string
	if strings.Contains(auth, ":") {
		split := strings.SplitN(auth, ":", 2)
		username = split[0]
		password = split[1]
	} else {
		username = auth
	}

	return &HttpBasicAuth{
		Username: username,
		Password: password,
	}
}
