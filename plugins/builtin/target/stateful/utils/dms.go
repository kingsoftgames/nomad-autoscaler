package utils

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
