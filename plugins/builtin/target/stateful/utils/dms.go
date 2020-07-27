package utils

// Nodes is used to query node-related API endpoints
type Dms struct {
	client *DmsApiClient
}
type DmsNodes struct {
	Nodes map[string]bool
}

// List is used to list out all of the nodes
func (n *Dms) List(q *QueryOptions) (*DmsNodes, *QueryMeta, error) {
	var resp DmsNodes
	qm, err := n.client.query("/v1/nodes", &resp, q)
	if err != nil {
		return nil, nil, err
	}
	return &resp, qm, nil
}

// Nodes returns a handle on the node endpoints.
func (c *DmsApiClient) Dms() *Dms {
	return &Dms{client: c}
}
