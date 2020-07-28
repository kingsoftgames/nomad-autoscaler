package utils

import (
	"bytes"
	"compress/gzip"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gorilla/websocket"
	cleanhttp "github.com/hashicorp/go-cleanhttp"
	rootcerts "github.com/hashicorp/go-rootcerts"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

var (
	// ClientConnTimeout is the timeout applied when attempting to contact a
	// client directly before switching to a connection through the Nomad
	// server.
	ClientConnTimeout = 1 * time.Second
)

const (
	// AllNamespacesNamespace is a sentinel Namespace value to indicate that api should search for
	// jobs and allocations in all the namespaces the requester can access.
	AllNamespacesNamespace = "*"
)

// HttpBasicAuth is used to authenticate http client with HTTP Basic Authentication
type HttpBasicAuth struct {
	// Username to use for HTTP Basic Authentication
	Username string

	// Password to use for HTTP Basic Authentication
	Password string
}

// DmsApiConfig is used to configure the creation of a client
type DmsApiConfig struct {
	// Address is the address of the Nomad agent
	Address string
	// SecretID to use. This can be overwritten per request.
	SecretID string

	// HttpClient is the client to use. Default will be used if not provided.
	//
	// If set, it expected to be configured for tls already, and TLSConfig is ignored.
	// You may use ConfigureTLS() function to aid with initialization.
	HttpClient *http.Client

	// HttpAuth is the auth info to use for http access.
	HttpAuth *HttpBasicAuth

	// WaitTime limits how long a Watch will block. If not provided,
	// the agent default values will be used.
	WaitTime time.Duration

	// TLSConfig provides the various TLS related configurations for the http
	// client.
	//
	// TLSConfig is ignored if HttpClient is set.
	TLSConfig *TLSConfig
}

// ClientConfig copies the configuration with a new client address, region, and
// whether the client has TLS enabled.
func (c *DmsApiConfig) ClientConfig(address string, tlsEnabled bool) *DmsApiConfig {
	scheme := "http"
	if tlsEnabled {
		scheme = "https"
	}
	config := &DmsApiConfig{
		Address:    fmt.Sprintf("%s://%s", scheme, address),
		HttpClient: c.HttpClient,
		SecretID:   c.SecretID,
		HttpAuth:   c.HttpAuth,
		WaitTime:   c.WaitTime,
		TLSConfig:  c.TLSConfig.Copy(),
	}

	// Update the tls server name for connecting to a client
	if tlsEnabled && config.TLSConfig != nil {
		config.TLSConfig.TLSServerName = fmt.Sprintf("client.nomad")
	}

	return config
}

// TLSConfig contains the parameters needed to configure TLS on the HTTP client
// used to communicate with Nomad.
type TLSConfig struct {
	// CACert is the path to a PEM-encoded CA cert file to use to verify the
	// Nomad server SSL certificate.
	CACert string

	// CAPath is the path to a directory of PEM-encoded CA cert files to verify
	// the Nomad server SSL certificate.
	CAPath string

	// CACertPem is the PEM-encoded CA cert to use to verify the Nomad server
	// SSL certificate.
	CACertPEM []byte

	// ClientCert is the path to the certificate for Nomad communication
	ClientCert string

	// ClientCertPEM is the PEM-encoded certificate for Nomad communication
	ClientCertPEM []byte

	// ClientKey is the path to the private key for Nomad communication
	ClientKey string

	// ClientKeyPEM is the PEM-encoded private key for Nomad communication
	ClientKeyPEM []byte

	// TLSServerName, if set, is used to set the SNI host when connecting via
	// TLS.
	TLSServerName string

	// Insecure enables or disables SSL verification
	Insecure bool
}

func (t *TLSConfig) Copy() *TLSConfig {
	if t == nil {
		return nil
	}

	nt := new(TLSConfig)
	*nt = *t
	return nt
}

func defaultHttpClient() *http.Client {
	httpClient := cleanhttp.DefaultClient()
	transport := httpClient.Transport.(*http.Transport)
	transport.TLSHandshakeTimeout = 10 * time.Second
	transport.TLSClientConfig = &tls.Config{
		MinVersion: tls.VersionTLS12,
	}

	return httpClient
}

// DefaultConfig returns a default configuration for the client
func DefaultConfig() *DmsApiConfig {
	config := &DmsApiConfig{
		Address:   "http://127.0.0.1:4646",
		TLSConfig: &TLSConfig{},
	}
	if addr := os.Getenv("DMS_ADDR"); addr != "" {
		config.Address = addr
	}
	if auth := os.Getenv("DMS_HTTP_AUTH"); auth != "" {
		var username, password string
		if strings.Contains(auth, ":") {
			split := strings.SplitN(auth, ":", 2)
			username = split[0]
			password = split[1]
		} else {
			username = auth
		}

		config.HttpAuth = &HttpBasicAuth{
			Username: username,
			Password: password,
		}
	}

	// Read TLS specific env vars
	if v := os.Getenv("DMS_CACERT"); v != "" {
		config.TLSConfig.CACert = v
	}
	if v := os.Getenv("DMS_CAPATH"); v != "" {
		config.TLSConfig.CAPath = v
	}
	if v := os.Getenv("DMS_CLIENT_CERT"); v != "" {
		config.TLSConfig.ClientCert = v
	}
	if v := os.Getenv("DMS_CLIENT_KEY"); v != "" {
		config.TLSConfig.ClientKey = v
	}
	if v := os.Getenv("DMS_TLS_SERVER_NAME"); v != "" {
		config.TLSConfig.TLSServerName = v
	}
	if v := os.Getenv("DMS_SKIP_VERIFY"); v != "" {
		if insecure, err := strconv.ParseBool(v); err == nil {
			config.TLSConfig.Insecure = insecure
		}
	}
	if v := os.Getenv("DMS_TOKEN"); v != "" {
		config.SecretID = v
	}
	return config
}

// cloneWithTimeout returns a cloned httpClient with set timeout if positive;
// otherwise, returns the same client
func cloneWithTimeout(httpClient *http.Client, t time.Duration) (*http.Client, error) {
	if httpClient == nil {
		return nil, fmt.Errorf("nil HTTP client")
	} else if httpClient.Transport == nil {
		return nil, fmt.Errorf("nil HTTP client transport")
	}

	if t.Nanoseconds() < 0 {
		return httpClient, nil
	}

	tr, ok := httpClient.Transport.(*http.Transport)
	if !ok {
		return nil, fmt.Errorf("unexpected HTTP transport: %T", httpClient.Transport)
	}

	// copy all public fields, to avoid copying transient state and locks
	ntr := &http.Transport{
		Proxy:                  tr.Proxy,
		DialContext:            tr.DialContext,
		Dial:                   tr.Dial,
		DialTLS:                tr.DialTLS,
		TLSClientConfig:        tr.TLSClientConfig,
		TLSHandshakeTimeout:    tr.TLSHandshakeTimeout,
		DisableKeepAlives:      tr.DisableKeepAlives,
		DisableCompression:     tr.DisableCompression,
		MaxIdleConns:           tr.MaxIdleConns,
		MaxIdleConnsPerHost:    tr.MaxIdleConnsPerHost,
		MaxConnsPerHost:        tr.MaxConnsPerHost,
		IdleConnTimeout:        tr.IdleConnTimeout,
		ResponseHeaderTimeout:  tr.ResponseHeaderTimeout,
		ExpectContinueTimeout:  tr.ExpectContinueTimeout,
		TLSNextProto:           tr.TLSNextProto,
		ProxyConnectHeader:     tr.ProxyConnectHeader,
		MaxResponseHeaderBytes: tr.MaxResponseHeaderBytes,
	}

	// apply timeout
	ntr.DialContext = (&net.Dialer{
		Timeout:   t,
		KeepAlive: 30 * time.Second,
	}).DialContext

	// clone http client with new transport
	nc := *httpClient
	nc.Transport = ntr
	return &nc, nil
}

// ConfigureTLS applies a set of TLS configurations to the the HTTP client.
func ConfigureTLS(httpClient *http.Client, tlsConfig *TLSConfig) error {
	if tlsConfig == nil {
		return nil
	}
	if httpClient == nil {
		return fmt.Errorf("config HTTP DmsApiClient must be set")
	}

	var clientCert tls.Certificate
	foundClientCert := false
	if tlsConfig.ClientCert != "" || tlsConfig.ClientKey != "" {
		if tlsConfig.ClientCert != "" && tlsConfig.ClientKey != "" {
			var err error
			clientCert, err = tls.LoadX509KeyPair(tlsConfig.ClientCert, tlsConfig.ClientKey)
			if err != nil {
				return err
			}
			foundClientCert = true
		} else {
			return fmt.Errorf("Both client cert and client key must be provided")
		}
	} else if len(tlsConfig.ClientCertPEM) != 0 || len(tlsConfig.ClientKeyPEM) != 0 {
		if len(tlsConfig.ClientCertPEM) != 0 && len(tlsConfig.ClientKeyPEM) != 0 {
			var err error
			clientCert, err = tls.X509KeyPair(tlsConfig.ClientCertPEM, tlsConfig.ClientKeyPEM)
			if err != nil {
				return err
			}
			foundClientCert = true
		} else {
			return fmt.Errorf("Both client cert and client key must be provided")
		}
	}

	clientTLSConfig := httpClient.Transport.(*http.Transport).TLSClientConfig
	rootConfig := &rootcerts.Config{
		CAFile:        tlsConfig.CACert,
		CAPath:        tlsConfig.CAPath,
		CACertificate: tlsConfig.CACertPEM,
	}
	if err := rootcerts.ConfigureTLS(clientTLSConfig, rootConfig); err != nil {
		return err
	}

	clientTLSConfig.InsecureSkipVerify = tlsConfig.Insecure

	if foundClientCert {
		clientTLSConfig.Certificates = []tls.Certificate{clientCert}
	}
	if tlsConfig.TLSServerName != "" {
		clientTLSConfig.ServerName = tlsConfig.TLSServerName
	}

	return nil
}

// DmsApiClient provides a client to the Nomad API
type DmsApiClient struct {
	httpClient *http.Client
	config     DmsApiConfig
}

// NewClient returns a new client
func NewDmsApiClient(config *DmsApiConfig) (*DmsApiClient, error) {
	// bootstrap the config
	defConfig := DefaultConfig()

	if config.Address == "" {
		config.Address = defConfig.Address
	} else if _, err := url.Parse(config.Address); err != nil {
		return nil, fmt.Errorf("invalid address '%s': %v", config.Address, err)
	}

	httpClient := config.HttpClient
	if httpClient == nil {
		httpClient = defaultHttpClient()
		if err := ConfigureTLS(httpClient, config.TLSConfig); err != nil {
			return nil, err
		}
	}

	client := &DmsApiClient{
		config:     *config,
		httpClient: httpClient,
	}
	return client, nil
}

// Address return the address of the Nomad agent
func (c *DmsApiClient) Address() string {
	return c.config.Address
}

// GetNodeClient returns a new DmsApiClient that will dial the specified node. If the
// QueryOptions is set, its region will be used.
func (c *DmsApiClient) GetDmsClient(address string, tlsEnabled bool) (*DmsApiClient, error) {
	return c.getDmsClientImpl(address, tlsEnabled, -1)
}

// GetNodeClientWithTimeout returns a new DmsApiClient that will dial the specified
// node using the specified timeout. If the QueryOptions is set, its region will
// be used.
func (c *DmsApiClient) GetDmsClientWithTimeout(
	addr string, tlsEnabled bool, timeout time.Duration) (*DmsApiClient, error) {
	return c.getDmsClientImpl(addr, tlsEnabled, timeout)
}

// getNodeClientImpl is the implementation of creating a API client for
// contacting a node. It takes a function to lookup the node such that it can be
// mocked during tests.
func (c *DmsApiClient) getDmsClientImpl(HTTPAddr string, TLSEnabled bool, timeout time.Duration) (*DmsApiClient, error) {

	// Get an API client for the node
	conf := c.config.ClientConfig(HTTPAddr, TLSEnabled)

	// set timeout - preserve old behavior where errors are ignored and use untimed one
	httpClient, err := cloneWithTimeout(c.httpClient, timeout)
	// on error, fallback to using current http client
	if err != nil {
		httpClient = c.httpClient
	}
	conf.HttpClient = httpClient

	return NewDmsApiClient(conf)
}

// SetSecretID sets the ACL token secret for API requests.
func (c *DmsApiClient) SetSecretID(secretID string) {
	c.config.SecretID = secretID
}

// request is used to help build up a request
type request struct {
	config *DmsApiConfig
	method string
	url    *url.URL
	params url.Values
	token  string
	body   io.Reader
	obj    interface{}
}

// durToMsec converts a duration to a millisecond specified string
func durToMsec(dur time.Duration) string {
	return fmt.Sprintf("%dms", dur/time.Millisecond)
}

// toHTTP converts the request to an HTTP request
func (r *request) toHTTP() (*http.Request, error) {
	// Encode the query parameters
	r.url.RawQuery = r.params.Encode()

	// Check if we should encode the body
	if r.body == nil && r.obj != nil {
		if b, err := encodeBody(r.obj); err != nil {
			return nil, err
		} else {
			r.body = b
		}
	}

	// Create the HTTP request
	req, err := http.NewRequest(r.method, r.url.RequestURI(), r.body)
	if err != nil {
		return nil, err
	}

	// Optionally configure HTTP basic authentication
	if r.url.User != nil {
		username := r.url.User.Username()
		password, _ := r.url.User.Password()
		req.SetBasicAuth(username, password)
	} else if r.config.HttpAuth != nil {
		req.SetBasicAuth(r.config.HttpAuth.Username, r.config.HttpAuth.Password)
	}

	req.Header.Add("Accept-Encoding", "gzip")
	if r.token != "" {
		req.Header.Set("X-Nomad-Token", r.token)
	}

	req.URL.Host = r.url.Host
	req.URL.Scheme = r.url.Scheme
	req.Host = r.url.Host
	return req, nil
}

// newRequest is used to create a new request
func (c *DmsApiClient) newRequest(method, path string) (*request, error) {
	base, _ := url.Parse(c.config.Address)
	u, err := url.Parse(path)
	if err != nil {
		return nil, err
	}
	r := &request{
		config: &c.config,
		method: method,
		url: &url.URL{
			Scheme:  base.Scheme,
			User:    base.User,
			Host:    base.Host,
			Path:    u.Path,
			RawPath: u.RawPath,
		},
		params: make(map[string][]string),
	}

	if c.config.WaitTime != 0 {
		r.params.Set("wait", durToMsec(r.config.WaitTime))
	}
	if c.config.SecretID != "" {
		r.token = r.config.SecretID
	}

	// Add in the query parameters, if any
	for key, values := range u.Query() {
		for _, value := range values {
			r.params.Add(key, value)
		}
	}

	return r, nil
}

// multiCloser is to wrap a ReadCloser such that when close is called, multiple
// Closes occur.
type multiCloser struct {
	reader       io.Reader
	inorderClose []io.Closer
}

func (m *multiCloser) Close() error {
	for _, c := range m.inorderClose {
		if err := c.Close(); err != nil {
			return err
		}
	}
	return nil
}

func (m *multiCloser) Read(p []byte) (int, error) {
	return m.reader.Read(p)
}

// doRequest runs a request with our client
func (c *DmsApiClient) doRequest(r *request) (*http.Response, error) {
	req, err := r.toHTTP()
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)

	// If the response is compressed, we swap the body's reader.
	if resp != nil && resp.Header != nil {
		var reader io.ReadCloser
		switch resp.Header.Get("Content-Encoding") {
		case "gzip":
			greader, err := gzip.NewReader(resp.Body)
			if err != nil {
				return nil, err
			}

			// The gzip reader doesn't close the wrapped reader so we use
			// multiCloser.
			reader = &multiCloser{
				reader:       greader,
				inorderClose: []io.Closer{greader, resp.Body},
			}
		default:
			reader = resp.Body
		}
		resp.Body = reader
	}

	return resp, err
}

// rawQuery makes a GET request to the specified endpoint but returns just the
// response body.
func (c *DmsApiClient) rawQuery(endpoint string) (io.ReadCloser, error) {
	r, err := c.newRequest("GET", endpoint)
	if err != nil {
		return nil, err
	}

	resp, err := requireOK(c.doRequest(r))
	if err != nil {
		return nil, err
	}

	return resp.Body, nil
}

// websocket makes a websocket request to the specific endpoint
func (c *DmsApiClient) websocket(endpoint string) (*websocket.Conn, *http.Response, error) {

	transport, ok := c.httpClient.Transport.(*http.Transport)
	if !ok {
		return nil, nil, fmt.Errorf("unsupported transport")
	}
	dialer := websocket.Dialer{
		ReadBufferSize:   4096,
		WriteBufferSize:  4096,
		HandshakeTimeout: c.httpClient.Timeout,

		// values to inherit from http client configuration
		NetDial:         transport.Dial,
		NetDialContext:  transport.DialContext,
		Proxy:           transport.Proxy,
		TLSClientConfig: transport.TLSClientConfig,
	}

	// build request object for header and parameters
	r, err := c.newRequest("GET", endpoint)
	if err != nil {
		return nil, nil, err
	}

	rhttp, err := r.toHTTP()
	if err != nil {
		return nil, nil, err
	}

	// convert scheme
	wsScheme := ""
	switch rhttp.URL.Scheme {
	case "http":
		wsScheme = "ws"
	case "https":
		wsScheme = "wss"
	default:
		return nil, nil, fmt.Errorf("unsupported scheme: %v", rhttp.URL.Scheme)
	}
	rhttp.URL.Scheme = wsScheme

	conn, resp, err := dialer.Dial(rhttp.URL.String(), rhttp.Header)

	// check resp status code, as it's more informative than handshake error we get from ws library
	if resp != nil && resp.StatusCode != 101 {
		var buf bytes.Buffer

		if resp.Header.Get("Content-Encoding") == "gzip" {
			greader, err := gzip.NewReader(resp.Body)
			if err != nil {
				return nil, nil, fmt.Errorf("Unexpected response code: %d", resp.StatusCode)
			}
			io.Copy(&buf, greader)
		} else {
			io.Copy(&buf, resp.Body)
		}
		resp.Body.Close()

		return nil, nil, fmt.Errorf("Unexpected response code: %d (%s)", resp.StatusCode, buf.Bytes())
	}

	return conn, resp, err
}

// query is used to do a GET request against an endpoint
// and deserialize the response into an interface using
// standard Nomad conventions.
func (c *DmsApiClient) query(endpoint string, out interface{}) error {
	r, err := c.newRequest("GET", endpoint)
	if err != nil {
		return err
	}

	resp, err := requireOK(c.doRequest(r))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if err := decodeBody(resp, out); err != nil {
		return err
	}
	return nil
}

// putQuery is used to do a PUT request when doing a read against an endpoint
// and deserialize the response into an interface using standard Nomad
// conventions.
func (c *DmsApiClient) putQuery(endpoint string, in, out interface{}) error {
	r, err := c.newRequest("PUT", endpoint)
	if err != nil {
		return err
	}

	r.obj = in
	resp, err := requireOK(c.doRequest(r))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if err := decodeBody(resp, out); err != nil {
		return err
	}
	return nil
}

// write is used to do a PUT request against an endpoint
// and serialize/deserialized using the standard Nomad conventions.
func (c *DmsApiClient) write(endpoint string, in, out interface{}) error {
	r, err := c.newRequest("PUT", endpoint)
	if err != nil {
		return err
	}

	r.obj = in
	resp, err := requireOK(c.doRequest(r))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if out != nil {
		if err := decodeBody(resp, &out); err != nil {
			return err
		}
	}
	return nil
}

// delete is used to do a DELETE request against an endpoint
// and serialize/deserialized using the standard Nomad conventions.
func (c *DmsApiClient) delete(endpoint string, out interface{}) error {
	r, err := c.newRequest("DELETE", endpoint)
	if err != nil {
		return err
	}

	resp, err := requireOK(c.doRequest(r))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if out != nil {
		if err := decodeBody(resp, &out); err != nil {
			return err
		}
	}
	return nil
}

// decodeBody is used to JSON decode a body
func decodeBody(resp *http.Response, out interface{}) error {
	switch resp.ContentLength {
	case 0:
		if out == nil {
			return nil
		}
		return errors.New("Got 0 byte response with non-nil decode object")
	default:
		dec := json.NewDecoder(resp.Body)
		return dec.Decode(out)
	}
}

// encodeBody prepares the reader to serve as the request body.
//
// Returns the `obj` input if it is a raw io.Reader object; otherwise
// returns a reader of the json format of the passed argument.
func encodeBody(obj interface{}) (io.Reader, error) {
	if reader, ok := obj.(io.Reader); ok {
		return reader, nil
	}

	buf := bytes.NewBuffer(nil)
	enc := json.NewEncoder(buf)
	if err := enc.Encode(obj); err != nil {
		return nil, err
	}
	return buf, nil
}

// requireOK is used to wrap doRequest and check for a 200
func requireOK(resp *http.Response, e error) (*http.Response, error) {
	if e != nil {
		if resp != nil {
			resp.Body.Close()
		}
		return nil, e
	}
	if resp.StatusCode != 200 {
		var buf bytes.Buffer
		io.Copy(&buf, resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("Unexpected response code: %d (%s)", resp.StatusCode, buf.Bytes())
	}
	return resp, nil
}
