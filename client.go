package req

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/xml"
	"errors"
	"github.com/imroc/req/v2/internal/util"
	"golang.org/x/net/publicsuffix"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/cookiejar"
	urlpkg "net/url"
	"os"
	"strings"
	"time"
)

// DefaultClient returns the global default Client.
func DefaultClient() *Client {
	return defaultClient
}

// SetDefaultClient override the global default Client.
func SetDefaultClient(c *Client) {
	if c != nil {
		defaultClient = c
	}
}

var defaultClient *Client = C()

// Client is the req's http client.
type Client struct {
	BaseURL               string
	PathParams            map[string]string
	QueryParams           urlpkg.Values
	Headers               http.Header
	Cookies               []*http.Cookie
	FormData              urlpkg.Values
	DebugLog              bool
	AllowGetMethodPayload bool

	jsonMarshal             func(v interface{}) ([]byte, error)
	jsonUnmarshal           func(data []byte, v interface{}) error
	xmlMarshal              func(v interface{}) ([]byte, error)
	xmlUnmarshal            func(data []byte, v interface{}) error
	trace                   bool
	outputDirectory         string
	disableAutoReadResponse bool
	scheme                  string
	log                     Logger
	t                       *Transport
	t2                      *http2Transport
	dumpOptions             *DumpOptions
	httpClient              *http.Client
	jsonDecoder             *json.Decoder
	beforeRequest           []RequestMiddleware
	udBeforeRequest         []RequestMiddleware
	afterResponse           []ResponseMiddleware
}

func cloneHeaders(hdrs http.Header) http.Header {
	if hdrs == nil {
		return nil
	}
	h := make(http.Header)
	for k, vs := range hdrs {
		for _, v := range vs {
			h.Add(k, v)
		}
	}
	return h
}

func cloneUrlValues(v urlpkg.Values) urlpkg.Values {
	if v == nil {
		return nil
	}
	vv := make(urlpkg.Values)
	for key, values := range v {
		for _, value := range values {
			vv.Add(key, value)
		}
	}
	return vv
}

func cloneMap(h map[string]string) map[string]string {
	if h == nil {
		return nil
	}
	m := make(map[string]string)
	for k, v := range h {
		m[k] = v
	}
	return m
}

func R() *Request {
	return defaultClient.R()
}

// R create a new request.
func (c *Client) R() *Request {
	req := &http.Request{
		Header:     make(http.Header),
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
	}
	return &Request{
		client:     c,
		RawRequest: req,
	}
}

func SetCommonFormDataFromValues(data urlpkg.Values) *Client {
	return defaultClient.SetCommonFormDataFromValues(data)
}

func (c *Client) SetCommonFormDataFromValues(data urlpkg.Values) *Client {
	if c.FormData == nil {
		c.FormData = urlpkg.Values{}
	}
	for k, v := range data {
		for _, kv := range v {
			c.FormData.Add(k, kv)
		}
	}
	return c
}

func SetCommonFormData(data map[string]string) *Client {
	return defaultClient.SetCommonFormData(data)
}

func (c *Client) SetCommonFormData(data map[string]string) *Client {
	if c.FormData == nil {
		c.FormData = urlpkg.Values{}
	}
	for k, v := range data {
		c.FormData.Set(k, v)
	}
	return c
}

func SetBaseURL(u string) *Client {
	return defaultClient.SetBaseURL(u)
}

func (c *Client) SetBaseURL(u string) *Client {
	c.BaseURL = strings.TrimRight(u, "/")
	return c
}

func SetOutputDirectory(dir string) *Client {
	return defaultClient.SetOutputDirectory(dir)
}

func (c *Client) SetOutputDirectory(dir string) *Client {
	c.outputDirectory = dir
	return c
}

func SetCertFromFile(certFile, keyFile string) *Client {
	return defaultClient.SetCertFromFile(certFile, keyFile)
}

// SetCertFromFile helps to set client certificates from cert and key file
func (c *Client) SetCertFromFile(certFile, keyFile string) *Client {
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		c.log.Errorf("failed to load client cert: %v", err)
		return c
	}
	config := c.tlsConfig()
	config.Certificates = append(config.Certificates, cert)
	return c
}

func SetCerts(certs ...tls.Certificate) *Client {
	return defaultClient.SetCerts(certs...)
}

// SetCerts helps to set client certificates
func (c *Client) SetCerts(certs ...tls.Certificate) *Client {
	config := c.tlsConfig()
	config.Certificates = append(config.Certificates, certs...)
	return c
}

func (c *Client) appendRootCertData(data []byte) {
	config := c.tlsConfig()
	if config.RootCAs == nil {
		config.RootCAs = x509.NewCertPool()
	}
	config.RootCAs.AppendCertsFromPEM(data)
	return
}

func SetRootCertFromString(pemContent string) *Client {
	return defaultClient.SetRootCertFromString(pemContent)
}

// SetRootCertFromString helps to set root CA cert from string
func (c *Client) SetRootCertFromString(pemContent string) *Client {
	c.appendRootCertData([]byte(pemContent))
	return c
}

func SetRootCertsFromFile(pemFiles ...string) *Client {
	return defaultClient.SetRootCertsFromFile(pemFiles...)
}

// SetRootCertsFromFile helps to set root certs from files
func (c *Client) SetRootCertsFromFile(pemFiles ...string) *Client {
	for _, pemFile := range pemFiles {
		rootPemData, err := ioutil.ReadFile(pemFile)
		if err != nil {
			c.log.Errorf("failed to read root cert file: %v", err)
			return c
		}
		c.appendRootCertData(rootPemData)
	}
	return c
}

func (c *Client) tlsConfig() *tls.Config {
	if c.t.TLSClientConfig == nil {
		c.t.TLSClientConfig = &tls.Config{}
	}
	return c.t.TLSClientConfig
}

func (c *Client) defaultCheckRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return errors.New("stopped after 10 redirects")
	}
	if c.DebugLog {
		c.log.Debugf("<redirect> %s %s", req.Method, req.URL.String())
	}
	return nil
}

func SetRedirectPolicy(policies ...RedirectPolicy) *Client {
	return defaultClient.SetRedirectPolicy(policies...)
}

// SetRedirectPolicy helps to set the RedirectPolicy
func (c *Client) SetRedirectPolicy(policies ...RedirectPolicy) *Client {
	if len(policies) == 0 {
		return c
	}
	c.httpClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		for _, f := range policies {
			if f == nil {
				continue
			}
			err := f(req, via)
			if err != nil {
				return err
			}
		}
		if c.DebugLog {
			c.log.Debugf("<redirect> %s %s", req.Method, req.URL.String())
		}
		return nil
	}
	return c
}

func DisableKeepAlives(disable bool) *Client {
	return defaultClient.DisableKeepAlives(disable)
}

func (c *Client) DisableKeepAlives(disable bool) *Client {
	c.t.DisableKeepAlives = disable
	return c
}

func DisableCompression(disable bool) *Client {
	return defaultClient.DisableCompression(disable)
}

func (c *Client) DisableCompression(disable bool) *Client {
	c.t.DisableCompression = disable
	return c
}

func SetTLSClientConfig(conf *tls.Config) *Client {
	return defaultClient.SetTLSClientConfig(conf)
}

func (c *Client) SetTLSClientConfig(conf *tls.Config) *Client {
	c.t.TLSClientConfig = conf
	return c
}

func SetCommonQueryParams(params map[string]string) *Client {
	return defaultClient.SetCommonQueryParams(params)
}

func (c *Client) SetCommonQueryParams(params map[string]string) *Client {
	for k, v := range params {
		c.SetCommonQueryParam(k, v)
	}
	return c
}

func SetCommonQueryParam(key, value string) *Client {
	return defaultClient.SetCommonQueryParam(key, value)
}

func (c *Client) SetCommonQueryParam(key, value string) *Client {
	if c.QueryParams == nil {
		c.QueryParams = make(urlpkg.Values)
	}
	c.QueryParams.Set(key, value)
	return c
}

func SetCommonQueryString(query string) *Client {
	return defaultClient.SetCommonQueryString(query)
}

func (c *Client) SetCommonQueryString(query string) *Client {
	params, err := urlpkg.ParseQuery(strings.TrimSpace(query))
	if err == nil {
		if c.QueryParams == nil {
			c.QueryParams = make(urlpkg.Values)
		}
		for p, v := range params {
			for _, pv := range v {
				c.QueryParams.Add(p, pv)
			}
		}
	} else {
		c.log.Warnf("failed to parse query string (%s): %v", query, err)
	}
	return c
}

func SetCommonCookie(hc *http.Cookie) *Client {
	return defaultClient.SetCommonCookie(hc)
}

func (c *Client) SetCommonCookie(hc *http.Cookie) *Client {
	c.Cookies = append(c.Cookies, hc)
	return c
}

func SetCommonCookies(cs []*http.Cookie) *Client {
	return defaultClient.SetCommonCookies(cs)
}

func (c *Client) SetCommonCookies(cs []*http.Cookie) *Client {
	c.Cookies = append(c.Cookies, cs...)
	return c
}

func EnableDebugLog(enable bool) *Client {
	return defaultClient.EnableDebugLog(enable)
}

func (c *Client) EnableDebugLog(enable bool) *Client {
	c.DebugLog = enable
	return c
}

// DevMode is a global wrapper method for default client.
func DevMode() *Client {
	return defaultClient.DevMode()
}

// DevMode enables:
// 1. Dump content of all requests and responses to see details.
// 2. Output debug log for deeper insights.
// 3. Trace all requests, so you can get trace info to analyze performance.
// 4. Set User-Agent to pretend to be a web browser, avoid returning abnormal data from some sites.
func (c *Client) DevMode() *Client {
	return c.EnableDumpAll().
		EnableDebugLog(true).
		EnableTraceAll(true).
		SetUserAgent("Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/97.0.4692.71 Safari/537.36")
}

func SetScheme(scheme string) *Client {
	return defaultClient.SetScheme(scheme)
}

// SetScheme method sets custom scheme in the Resty client. It's way to override default.
// 		client.SetScheme("http")
func (c *Client) SetScheme(scheme string) *Client {
	if !util.IsStringEmpty(scheme) {
		c.scheme = strings.TrimSpace(scheme)
	}
	return c
}

func SetLogger(log Logger) *Client {
	return defaultClient.SetLogger(log)
}

// SetLogger set the logger for req, set to nil to disable logger.
func (c *Client) SetLogger(log Logger) *Client {
	if log == nil {
		c.log = &disableLogger{}
		return c
	}
	c.log = log
	return c
}

func (c *Client) getResponseOptions() *ResponseOptions {
	if c.t.ResponseOptions == nil {
		c.t.ResponseOptions = &ResponseOptions{}
	}
	return c.t.ResponseOptions
}

func SetTimeout(d time.Duration) *Client {
	return defaultClient.SetTimeout(d)
}

// SetTimeout set the timeout for all requests.
func (c *Client) SetTimeout(d time.Duration) *Client {
	c.httpClient.Timeout = d
	return c
}

func (c *Client) getDumpOptions() *DumpOptions {
	if c.dumpOptions == nil {
		c.dumpOptions = newDefaultDumpOptions()
	}
	return c.dumpOptions
}

func (c *Client) enableDump() {
	if c.t.dump != nil { // dump already started
		return
	}
	c.t.EnableDump(c.getDumpOptions())
}

func EnableDumpToFile(filename string) *Client {
	return defaultClient.EnableDumpToFile(filename)
}

// EnableDumpToFile indicates that the content should dump to the specified filename.
func (c *Client) EnableDumpToFile(filename string) *Client {
	file, err := os.Create(filename)
	if err != nil {
		c.log.Errorf("create dump file error: %v", err)
		return c
	}
	c.getDumpOptions().Output = file
	return c
}

func EnableDumpTo(output io.Writer) *Client {
	return defaultClient.EnableDumpTo(output)
}

// EnableDumpTo indicates that the content should dump to the specified destination.
func (c *Client) EnableDumpTo(output io.Writer) *Client {
	c.getDumpOptions().Output = output
	c.enableDump()
	return c
}

func EnableDumpAsync() *Client {
	return defaultClient.EnableDumpAsync()
}

// EnableDumpAsync indicates that the dump should be done asynchronously,
// can be used for debugging in production environment without
// affecting performance.
func (c *Client) EnableDumpAsync() *Client {
	o := c.getDumpOptions()
	o.Async = true
	c.enableDump()
	return c
}

func EnableDumpNoRequestBody() *Client {
	return defaultClient.EnableDumpNoRequestBody()
}

func (c *Client) EnableDumpNoRequestBody() *Client {
	o := c.getDumpOptions()
	o.ResponseHeader = true
	o.ResponseBody = true
	o.RequestBody = false
	o.RequestHeader = true
	c.enableDump()
	return c
}

func EnableDumpNoResponseBody() *Client {
	return defaultClient.EnableDumpNoResponseBody()
}

func (c *Client) EnableDumpNoResponseBody() *Client {
	o := c.getDumpOptions()
	o.ResponseHeader = true
	o.ResponseBody = false
	o.RequestBody = true
	o.RequestHeader = true
	c.enableDump()
	return c
}

func EnableDumpOnlyResponse() *Client {
	return defaultClient.EnableDumpOnlyResponse()
}

// EnableDumpOnlyResponse indicates that should dump the responses' head and response.
func (c *Client) EnableDumpOnlyResponse() *Client {
	o := c.getDumpOptions()
	o.ResponseHeader = true
	o.ResponseBody = true
	o.RequestBody = false
	o.RequestHeader = false
	c.enableDump()
	return c
}

func EnableDumpOnlyRequest() *Client {
	return defaultClient.EnableDumpOnlyRequest()
}

// EnableDumpOnlyRequest indicates that should dump the requests' head and response.
func (c *Client) EnableDumpOnlyRequest() *Client {
	o := c.getDumpOptions()
	o.RequestHeader = true
	o.RequestBody = true
	o.ResponseBody = false
	o.ResponseHeader = false
	c.enableDump()
	return c
}

func EnableDumpOnlyBody() *Client {
	return defaultClient.EnableDumpOnlyBody()
}

// EnableDumpOnlyBody indicates that should dump the body of requests and responses.
func (c *Client) EnableDumpOnlyBody() *Client {
	o := c.getDumpOptions()
	o.RequestBody = true
	o.ResponseBody = true
	o.RequestHeader = false
	o.ResponseHeader = false
	c.enableDump()
	return c
}

func EnableDumpOnlyHeader() *Client {
	return defaultClient.EnableDumpOnlyHeader()
}

// EnableDumpOnlyHeader indicates that should dump the head of requests and responses.
func (c *Client) EnableDumpOnlyHeader() *Client {
	o := c.getDumpOptions()
	o.RequestHeader = true
	o.ResponseHeader = true
	o.RequestBody = false
	o.ResponseBody = false
	c.enableDump()
	return c
}

func EnableDumpAll() *Client {
	return defaultClient.EnableDumpAll()
}

// EnableDumpAll indicates that should dump both requests and responses' head and body.
func (c *Client) EnableDumpAll() *Client {
	o := c.getDumpOptions()
	o.RequestHeader = true
	o.RequestBody = true
	o.ResponseHeader = true
	o.ResponseBody = true
	c.enableDump()
	return c
}

func NewRequest() *Request {
	return defaultClient.R()
}

// NewRequest is the alias of R()
func (c *Client) NewRequest() *Request {
	return c.R()
}

func DisableAutoReadResponse(disable bool) *Client {
	return defaultClient.DisableAutoReadResponse(disable)
}

func (c *Client) DisableAutoReadResponse(disable bool) *Client {
	c.disableAutoReadResponse = disable
	return c
}

func SetAutoDecodeContentType(contentTypes ...string) *Client {
	return defaultClient.SetAutoDecodeContentType(contentTypes...)
}

func (c *Client) SetAutoDecodeContentType(contentTypes ...string) *Client {
	opt := c.getResponseOptions()
	opt.AutoDecodeContentType = autoDecodeContentTypeFunc(contentTypes...)
	return c
}

func SetAutoDecodeAllTypeFunc(fn func(contentType string) bool) *Client {
	return defaultClient.SetAutoDecodeAllTypeFunc(fn)
}

func (c *Client) SetAutoDecodeAllTypeFunc(fn func(contentType string) bool) *Client {
	opt := c.getResponseOptions()
	opt.AutoDecodeContentType = fn
	return c
}

func SetAutoDecodeAllType() *Client {
	return defaultClient.SetAutoDecodeAllType()
}

// SetAutoDecodeAllType indicates that try autodetect and decode all content type.
func (c *Client) SetAutoDecodeAllType() *Client {
	opt := c.getResponseOptions()
	opt.AutoDecodeContentType = func(contentType string) bool {
		return true
	}
	return c
}

func DisableAutoDecode(disable bool) *Client {
	return defaultClient.DisableAutoDecode(disable)
}

// DisableAutoDecode disable auto detect charset and decode to utf-8
func (c *Client) DisableAutoDecode(disable bool) *Client {
	c.getResponseOptions().DisableAutoDecode = disable
	return c
}

func SetUserAgent(userAgent string) *Client {
	return defaultClient.SetUserAgent(userAgent)
}

// SetUserAgent set the "User-Agent" header for all requests.
func (c *Client) SetUserAgent(userAgent string) *Client {
	return c.SetCommonHeader(hdrUserAgentKey, userAgent)
}

func SetCommonBearerAuthToken(token string) *Client {
	return defaultClient.SetCommonBearerAuthToken(token)
}

func (c *Client) SetCommonBearerAuthToken(token string) *Client {
	return c.SetCommonHeader("Authorization", "Bearer "+token)
}

func SetCommonBasicAuth(username, password string) *Client {
	return defaultClient.SetCommonBasicAuth(username, password)
}

func (c *Client) SetCommonBasicAuth(username, password string) *Client {
	c.SetCommonHeader("Authorization", util.BasicAuthHeaderValue(username, password))
	return c
}

func SetCommonHeaders(hdrs map[string]string) *Client {
	return defaultClient.SetCommonHeaders(hdrs)
}

func (c *Client) SetCommonHeaders(hdrs map[string]string) *Client {
	for k, v := range hdrs {
		c.SetCommonHeader(k, v)
	}
	return c
}

func SetCommonHeader(key, value string) *Client {
	return defaultClient.SetCommonHeader(key, value)
}

// SetCommonHeader set the common header for all requests.
func (c *Client) SetCommonHeader(key, value string) *Client {
	if c.Headers == nil {
		c.Headers = make(http.Header)
	}
	c.Headers.Set(key, value)
	return c
}

func SetCommonContentType(ct string) *Client {
	return defaultClient.SetCommonContentType(ct)
}

func (c *Client) SetCommonContentType(ct string) *Client {
	c.SetCommonHeader(hdrContentTypeKey, ct)
	return c
}

func EnableDump(enable bool) *Client {
	return defaultClient.EnableDump(enable)
}

// EnableDump enables dump requests and responses,  allowing you
// to clearly see the content of all requests and responses，which
// is very convenient for debugging APIs.
func (c *Client) EnableDump(enable bool) *Client {
	if !enable {
		c.t.DisableDump()
		return c
	}
	c.enableDump()
	return c
}

func SetDumpOptions(opt *DumpOptions) *Client {
	return defaultClient.SetDumpOptions(opt)
}

// SetDumpOptions configures the underlying Transport's DumpOptions
func (c *Client) SetDumpOptions(opt *DumpOptions) *Client {
	if opt == nil {
		return c
	}
	c.dumpOptions = opt
	if c.t.dump != nil {
		c.t.dump.DumpOptions = opt
	}
	return c
}

func SetProxy(proxy func(*http.Request) (*urlpkg.URL, error)) *Client {
	return defaultClient.SetProxy(proxy)
}

// SetProxy set the proxy function.
func (c *Client) SetProxy(proxy func(*http.Request) (*urlpkg.URL, error)) *Client {
	c.t.Proxy = proxy
	return c
}

func OnBeforeRequest(m RequestMiddleware) *Client {
	return defaultClient.OnBeforeRequest(m)
}

func (c *Client) OnBeforeRequest(m RequestMiddleware) *Client {
	c.udBeforeRequest = append(c.udBeforeRequest, m)
	return c
}

func OnAfterResponse(m ResponseMiddleware) *Client {
	return defaultClient.OnAfterResponse(m)
}

func (c *Client) OnAfterResponse(m ResponseMiddleware) *Client {
	c.afterResponse = append(c.afterResponse, m)
	return c
}

func SetProxyURL(proxyUrl string) *Client {
	return defaultClient.SetProxyURL(proxyUrl)
}

func (c *Client) SetProxyURL(proxyUrl string) *Client {
	u, err := urlpkg.Parse(proxyUrl)
	if err != nil {
		c.log.Errorf("failed to parse proxy url %s: %v", proxyUrl, err)
		return c
	}
	c.t.Proxy = http.ProxyURL(u)
	return c
}

func EnableTraceAll(enable bool) *Client {
	return defaultClient.EnableTraceAll(enable)
}

func (c *Client) EnableTraceAll(enable bool) *Client {
	c.trace = enable
	return c
}

func SetCookieJar(jar http.CookieJar) *Client {
	return defaultClient.SetCookieJar(jar)
}

func (c *Client) SetCookieJar(jar http.CookieJar) *Client {
	c.httpClient.Jar = jar
	return c
}

func SetJsonMarshal(fn func(v interface{}) ([]byte, error)) *Client {
	return defaultClient.SetJsonMarshal(fn)
}

func (c *Client) SetJsonMarshal(fn func(v interface{}) ([]byte, error)) *Client {
	c.jsonMarshal = fn
	return c
}

func SetJsonUnmarshal(fn func(data []byte, v interface{}) error) *Client {
	return defaultClient.SetJsonUnmarshal(fn)
}

func (c *Client) SetJsonUnmarshal(fn func(data []byte, v interface{}) error) *Client {
	c.jsonUnmarshal = fn
	return c
}

func SetXmlMarshal(fn func(v interface{}) ([]byte, error)) *Client {
	return defaultClient.SetXmlMarshal(fn)
}

func (c *Client) SetXmlMarshal(fn func(v interface{}) ([]byte, error)) *Client {
	c.xmlMarshal = fn
	return c
}

func SetXmlUnmarshal(fn func(data []byte, v interface{}) error) *Client {
	return defaultClient.SetXmlUnmarshal(fn)
}

func (c *Client) SetXmlUnmarshal(fn func(data []byte, v interface{}) error) *Client {
	c.xmlUnmarshal = fn
	return c
}

func EnableAllowGetMethodPayload(a bool) *Client {
	return defaultClient.EnableAllowGetMethodPayload(a)
}

func (c *Client) EnableAllowGetMethodPayload(a bool) *Client {
	c.AllowGetMethodPayload = a
	return c
}

func (c *Client) isPayloadForbid(m string) bool {
	return (m == http.MethodGet && !c.AllowGetMethodPayload) || m == http.MethodHead || m == http.MethodOptions
}

// NewClient is the alias of C
func NewClient() *Client {
	return C()
}

// Clone copy and returns the Client
func (c *Client) Clone() *Client {
	t := c.t.Clone()
	t2, _ := http2ConfigureTransports(t)
	cc := *c.httpClient
	cc.Transport = t
	return &Client{
		httpClient:              &cc,
		t:                       t,
		t2:                      t2,
		dumpOptions:             c.dumpOptions.Clone(),
		jsonDecoder:             c.jsonDecoder,
		Headers:                 cloneHeaders(c.Headers),
		PathParams:              cloneMap(c.PathParams),
		QueryParams:             cloneUrlValues(c.QueryParams),
		BaseURL:                 c.BaseURL,
		scheme:                  c.scheme,
		log:                     c.log,
		beforeRequest:           c.beforeRequest,
		udBeforeRequest:         c.udBeforeRequest,
		disableAutoReadResponse: c.disableAutoReadResponse,
	}
}

// C create a new client.
func C() *Client {
	t := &Transport{
		ResponseOptions:       &ResponseOptions{},
		ForceAttemptHTTP2:     true,
		Proxy:                 http.ProxyFromEnvironment,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
	t2, _ := http2ConfigureTransports(t)
	jar, _ := cookiejar.New(&cookiejar.Options{PublicSuffixList: publicsuffix.List})
	httpClient := &http.Client{
		Transport: t,
		Jar:       jar,
		Timeout:   2 * time.Minute,
	}
	beforeRequest := []RequestMiddleware{
		parseRequestURL,
		parseRequestHeader,
		parseRequestCookie,
		parseRequestBody,
	}
	afterResponse := []ResponseMiddleware{
		parseResponseBody,
		handleDownload,
	}
	c := &Client{
		beforeRequest: beforeRequest,
		afterResponse: afterResponse,
		log:           createDefaultLogger(),
		httpClient:    httpClient,
		t:             t,
		t2:            t2,
		jsonMarshal:   json.Marshal,
		jsonUnmarshal: json.Unmarshal,
		xmlMarshal:    xml.Marshal,
		xmlUnmarshal:  xml.Unmarshal,
	}
	httpClient.CheckRedirect = c.defaultCheckRedirect

	t.Debugf = func(format string, v ...interface{}) {
		if c.DebugLog {
			c.log.Debugf(format, v...)
		}
	}
	return c
}

func setupRequest(r *Request) {
	setRequestURL(r.RawRequest, r.URL)
	setRequestHeaderAndCookie(r)
	setTrace(r)
}

func (c *Client) do(r *Request) (resp *Response, err error) {

	resp = &Response{}

	for _, f := range r.client.udBeforeRequest {
		if err = f(r.client, r); err != nil {
			return
		}
	}

	for _, f := range r.client.beforeRequest {
		if err = f(r.client, r); err != nil {
			return
		}
	}

	setupRequest(r)

	if c.DebugLog {
		c.log.Debugf("%s %s", r.RawRequest.Method, r.RawRequest.URL.String())
	}

	r.StartTime = time.Now()
	httpResponse, err := c.httpClient.Do(r.RawRequest)
	if err != nil {
		return
	}

	resp.Request = r
	resp.Response = httpResponse

	if !c.disableAutoReadResponse && !r.isSaveResponse { // auto read response body
		_, err = resp.ToBytes()
		if err != nil {
			return
		}
	}

	for _, f := range r.client.afterResponse {
		if err = f(r.client, resp); err != nil {
			return
		}
	}
	return
}

func setTrace(r *Request) {
	if r.trace == nil {
		if r.client.trace {
			r.trace = &clientTrace{}
		} else {
			return
		}
	}
	r.ctx = r.trace.createContext(r.Context())
	r.RawRequest = r.RawRequest.WithContext(r.ctx)
}

func setRequestHeaderAndCookie(r *Request) {
	for k, vs := range r.Headers {
		for _, v := range vs {
			r.RawRequest.Header.Add(k, v)
		}
	}
	for _, cookie := range r.Cookies {
		r.RawRequest.AddCookie(cookie)
	}
}

func setRequestURL(r *http.Request, url string) error {
	// The host's colon:port should be normalized. See Issue 14836.
	u, err := urlpkg.Parse(url)
	if err != nil {
		return err
	}
	u.Host = removeEmptyPort(u.Host)
	r.URL = u
	r.Host = u.Host
	return nil
}
