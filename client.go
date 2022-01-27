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
	beforeRequest           []RequestMiddleware
	udBeforeRequest         []RequestMiddleware
	afterResponse           []ResponseMiddleware
}

func cloneCookies(cookies []*http.Cookie) []*http.Cookie {
	if len(cookies) == 0 {
		return nil
	}
	c := make([]*http.Cookie, len(cookies))
	copy(c, cookies)
	return c
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

// TODO: change to generics function when generics are commonly used.
func cloneRequestMiddleware(m []RequestMiddleware) []RequestMiddleware {
	if len(m) == 0 {
		return nil
	}
	mm := make([]RequestMiddleware, len(m))
	copy(mm, m)
	return mm
}

func cloneResponseMiddleware(m []ResponseMiddleware) []ResponseMiddleware {
	if len(m) == 0 {
		return nil
	}
	mm := make([]ResponseMiddleware, len(m))
	copy(mm, m)
	return mm
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

// R is a global wrapper methods which delegated
// to the default client's R().
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

// SetCommonFormDataFromValues is a global wrapper methods which delegated
// to the default client's SetCommonFormDataFromValues.
func SetCommonFormDataFromValues(data urlpkg.Values) *Client {
	return defaultClient.SetCommonFormDataFromValues(data)
}

// SetCommonFormDataFromValues set the form data from url.Values for all requests which method allows payload.
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

// SetCommonFormData is a global wrapper methods which delegated
// to the default client's SetCommonFormData.
func SetCommonFormData(data map[string]string) *Client {
	return defaultClient.SetCommonFormData(data)
}

// SetCommonFormData set the form data from map for all requests which method allows payload.
func (c *Client) SetCommonFormData(data map[string]string) *Client {
	if c.FormData == nil {
		c.FormData = urlpkg.Values{}
	}
	for k, v := range data {
		c.FormData.Set(k, v)
	}
	return c
}

// SetBaseURL is a global wrapper methods which delegated
// to the default client's SetBaseURL.
func SetBaseURL(u string) *Client {
	return defaultClient.SetBaseURL(u)
}

// SetBaseURL set the default base url, will be used if request url is
// a relative url.
func (c *Client) SetBaseURL(u string) *Client {
	c.BaseURL = strings.TrimRight(u, "/")
	return c
}

// SetOutputDirectory is a global wrapper methods which delegated
// to the default client's SetOutputDirectory.
func SetOutputDirectory(dir string) *Client {
	return defaultClient.SetOutputDirectory(dir)
}

// SetOutputDirectory set output directory that response will be downloaded to.
func (c *Client) SetOutputDirectory(dir string) *Client {
	c.outputDirectory = dir
	return c
}

// SetCertFromFile is a global wrapper methods which delegated
// to the default client's SetCertFromFile.
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

// SetCerts is a global wrapper methods which delegated
// to the default client's SetCerts.
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

// SetRootCertFromString is a global wrapper methods which delegated
// to the default client's SetRootCertFromString.
func SetRootCertFromString(pemContent string) *Client {
	return defaultClient.SetRootCertFromString(pemContent)
}

// SetRootCertFromString helps to set root CA cert from string
func (c *Client) SetRootCertFromString(pemContent string) *Client {
	c.appendRootCertData([]byte(pemContent))
	return c
}

// SetRootCertsFromFile is a global wrapper methods which delegated
// to the default client's SetRootCertsFromFile.
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

// SetRedirectPolicy is a global wrapper methods which delegated
// to the default client's SetRedirectPolicy.
func SetRedirectPolicy(policies ...RedirectPolicy) *Client {
	return defaultClient.SetRedirectPolicy(policies...)
}

// SetRedirectPolicy helps to set the RedirectPolicy.
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

// DisableKeepAlives is a global wrapper methods which delegated
// to the default client's DisableKeepAlives.
func DisableKeepAlives(disable bool) *Client {
	return defaultClient.DisableKeepAlives(disable)
}

// DisableKeepAlives set to true disables HTTP keep-alives and
// will only use the connection to the server for a single
// HTTP request.
//
// This is unrelated to the similarly named TCP keep-alives.
func (c *Client) DisableKeepAlives(disable bool) *Client {
	c.t.DisableKeepAlives = disable
	return c
}

// DisableCompression is a global wrapper methods which delegated
// to the default client's DisableCompression.
func DisableCompression(disable bool) *Client {
	return defaultClient.DisableCompression(disable)
}

// DisableCompression set to true prevents the Transport from
// requesting compression with an "Accept-Encoding: gzip"
// request header when the Request contains no existing
// Accept-Encoding value. If the Transport requests gzip on
// its own and gets a gzipped response, it's transparently
// decoded in the Response.Body. However, if the user
// explicitly requested gzip it is not automatically
// uncompressed.
func (c *Client) DisableCompression(disable bool) *Client {
	c.t.DisableCompression = disable
	return c
}

// SetTLSClientConfig is a global wrapper methods which delegated
// to the default client's SetTLSClientConfig.
func SetTLSClientConfig(conf *tls.Config) *Client {
	return defaultClient.SetTLSClientConfig(conf)
}

// SetTLSClientConfig sets the client tls config.
func (c *Client) SetTLSClientConfig(conf *tls.Config) *Client {
	c.t.TLSClientConfig = conf
	return c
}

// SetCommonQueryParams is a global wrapper methods which delegated
// to the default client's SetCommonQueryParams.
func SetCommonQueryParams(params map[string]string) *Client {
	return defaultClient.SetCommonQueryParams(params)
}

// SetCommonQueryParams sets the URL query parameters with a map at client level.
func (c *Client) SetCommonQueryParams(params map[string]string) *Client {
	for k, v := range params {
		c.SetCommonQueryParam(k, v)
	}
	return c
}

// SetCommonQueryParam is a global wrapper methods which delegated
// to the default client's SetCommonQueryParam.
func SetCommonQueryParam(key, value string) *Client {
	return defaultClient.SetCommonQueryParam(key, value)
}

// SetCommonQueryParam set an URL query parameter with a key-value
// pair at client level.
func (c *Client) SetCommonQueryParam(key, value string) *Client {
	if c.QueryParams == nil {
		c.QueryParams = make(urlpkg.Values)
	}
	c.QueryParams.Set(key, value)
	return c
}

// SetCommonQueryString is a global wrapper methods which delegated
// to the default client's SetCommonQueryString.
func SetCommonQueryString(query string) *Client {
	return defaultClient.SetCommonQueryString(query)
}

// SetCommonQueryString set URL query parameters using the raw query string.
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

// SetCommonCookies is a global wrapper methods which delegated
// to the default client's SetCommonCookies.
func SetCommonCookies(cookies ...*http.Cookie) *Client {
	return defaultClient.SetCommonCookies(cookies...)
}

// SetCommonCookies set cookies at client level.
func (c *Client) SetCommonCookies(cookies ...*http.Cookie) *Client {
	c.Cookies = append(c.Cookies, cookies...)
	return c
}

// EnableDebugLog is a global wrapper methods which delegated
// to the default client's EnableDebugLog.
func EnableDebugLog(enable bool) *Client {
	return defaultClient.EnableDebugLog(enable)
}

// EnableDebugLog enables debug level log if set to true.
func (c *Client) EnableDebugLog(enable bool) *Client {
	c.DebugLog = enable
	return c
}

// DevMode is a global wrapper methods which delegated
// to the default client's DevMode.
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

// SetScheme is a global wrapper methods which delegated
// to the default client's SetScheme.
func SetScheme(scheme string) *Client {
	return defaultClient.SetScheme(scheme)
}

// SetScheme sets custom default scheme in the client, will be used when
// there is no scheme in the request url.
func (c *Client) SetScheme(scheme string) *Client {
	if !util.IsStringEmpty(scheme) {
		c.scheme = strings.TrimSpace(scheme)
	}
	return c
}

// SetLogger is a global wrapper methods which delegated
// to the default client's SetLogger.
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

// SetTimeout is a global wrapper methods which delegated
// to the default client's SetTimeout.
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

// EnableDumpToFile is a global wrapper methods which delegated
// to the default client's EnableDumpToFile.
func EnableDumpToFile(filename string) *Client {
	return defaultClient.EnableDumpToFile(filename)
}

// EnableDumpToFile enables dump and save to the specified filename.
func (c *Client) EnableDumpToFile(filename string) *Client {
	file, err := os.Create(filename)
	if err != nil {
		c.log.Errorf("create dump file error: %v", err)
		return c
	}
	c.getDumpOptions().Output = file
	c.enableDump()
	return c
}

// EnableDumpTo is a global wrapper methods which delegated
// to the default client's EnableDumpTo.
func EnableDumpTo(output io.Writer) *Client {
	return defaultClient.EnableDumpTo(output)
}

// EnableDumpTo enables dump and save to the specified io.Writer.
func (c *Client) EnableDumpTo(output io.Writer) *Client {
	c.getDumpOptions().Output = output
	c.enableDump()
	return c
}

// EnableDumpAsync is a global wrapper methods which delegated
// to the default client's EnableDumpAsync.
func EnableDumpAsync() *Client {
	return defaultClient.EnableDumpAsync()
}

// EnableDumpAsync enables dump and output asynchronously,
// can be used for debugging in production environment without
// affecting performance.
func (c *Client) EnableDumpAsync() *Client {
	o := c.getDumpOptions()
	o.Async = true
	c.enableDump()
	return c
}

// EnableDumpNoRequestBody is a global wrapper methods which delegated
// to the default client's EnableDumpNoRequestBody.
func EnableDumpNoRequestBody() *Client {
	return defaultClient.EnableDumpNoRequestBody()
}

// EnableDumpNoRequestBody enables dump with request body excluded, can be
// used in upload request to avoid dump the unreadable binary content.
func (c *Client) EnableDumpNoRequestBody() *Client {
	o := c.getDumpOptions()
	o.ResponseHeader = true
	o.ResponseBody = true
	o.RequestBody = false
	o.RequestHeader = true
	c.enableDump()
	return c
}

// EnableDumpNoResponseBody is a global wrapper methods which delegated
// to the default client's EnableDumpNoResponseBody.
func EnableDumpNoResponseBody() *Client {
	return defaultClient.EnableDumpNoResponseBody()
}

// EnableDumpNoResponseBody enables dump with response body excluded, can be
// used in download request to avoid dump the unreadable binary content.
func (c *Client) EnableDumpNoResponseBody() *Client {
	o := c.getDumpOptions()
	o.ResponseHeader = true
	o.ResponseBody = false
	o.RequestBody = true
	o.RequestHeader = true
	c.enableDump()
	return c
}

// EnableDumpOnlyResponse is a global wrapper methods which delegated
// to the default client's EnableDumpOnlyResponse.
func EnableDumpOnlyResponse() *Client {
	return defaultClient.EnableDumpOnlyResponse()
}

// EnableDumpOnlyResponse enables dump with only response included.
func (c *Client) EnableDumpOnlyResponse() *Client {
	o := c.getDumpOptions()
	o.ResponseHeader = true
	o.ResponseBody = true
	o.RequestBody = false
	o.RequestHeader = false
	c.enableDump()
	return c
}

// EnableDumpOnlyRequest is a global wrapper methods which delegated
// to the default client's EnableDumpOnlyRequest.
func EnableDumpOnlyRequest() *Client {
	return defaultClient.EnableDumpOnlyRequest()
}

// EnableDumpOnlyRequest enables dump with only request included.
func (c *Client) EnableDumpOnlyRequest() *Client {
	o := c.getDumpOptions()
	o.RequestHeader = true
	o.RequestBody = true
	o.ResponseBody = false
	o.ResponseHeader = false
	c.enableDump()
	return c
}

// EnableDumpOnlyBody is a global wrapper methods which delegated
// to the default client's EnableDumpOnlyBody.
func EnableDumpOnlyBody() *Client {
	return defaultClient.EnableDumpOnlyBody()
}

// EnableDumpOnlyBody enables dump with only body included.
func (c *Client) EnableDumpOnlyBody() *Client {
	o := c.getDumpOptions()
	o.RequestBody = true
	o.ResponseBody = true
	o.RequestHeader = false
	o.ResponseHeader = false
	c.enableDump()
	return c
}

// EnableDumpOnlyHeader is a global wrapper methods which delegated
// to the default client's EnableDumpOnlyHeader.
func EnableDumpOnlyHeader() *Client {
	return defaultClient.EnableDumpOnlyHeader()
}

// EnableDumpOnlyHeader enables dump with only header included.
func (c *Client) EnableDumpOnlyHeader() *Client {
	o := c.getDumpOptions()
	o.RequestHeader = true
	o.ResponseHeader = true
	o.RequestBody = false
	o.ResponseBody = false
	c.enableDump()
	return c
}

// EnableDumpAll is a global wrapper methods which delegated
// to the default client's EnableDumpAll.
func EnableDumpAll() *Client {
	return defaultClient.EnableDumpAll()
}

// EnableDumpAll enables dump with all content included,
// including both requests and responses' header and body
func (c *Client) EnableDumpAll() *Client {
	o := c.getDumpOptions()
	o.RequestHeader = true
	o.RequestBody = true
	o.ResponseHeader = true
	o.ResponseBody = true
	c.enableDump()
	return c
}

// NewRequest is a global wrapper methods which delegated
// to the default client's NewRequest.
func NewRequest() *Request {
	return defaultClient.R()
}

// NewRequest is the alias of R()
func (c *Client) NewRequest() *Request {
	return c.R()
}

// DisableAutoReadResponse is a global wrapper methods which delegated
// to the default client's DisableAutoReadResponse.
func DisableAutoReadResponse(disable bool) *Client {
	return defaultClient.DisableAutoReadResponse(disable)
}

// DisableAutoReadResponse disable read response body automatically if set to true.
func (c *Client) DisableAutoReadResponse(disable bool) *Client {
	c.disableAutoReadResponse = disable
	return c
}

// SetAutoDecodeContentType is a global wrapper methods which delegated
// to the default client's SetAutoDecodeContentType.
func SetAutoDecodeContentType(contentTypes ...string) *Client {
	return defaultClient.SetAutoDecodeContentType(contentTypes...)
}

// SetAutoDecodeContentType set the content types that will be auto-detected and decode
// to utf-8
func (c *Client) SetAutoDecodeContentType(contentTypes ...string) *Client {
	opt := c.getResponseOptions()
	opt.AutoDecodeContentType = autoDecodeContentTypeFunc(contentTypes...)
	return c
}

// SetAutoDecodeAllTypeFunc is a global wrapper methods which delegated
// to the default client's SetAutoDecodeAllTypeFunc.
func SetAutoDecodeAllTypeFunc(fn func(contentType string) bool) *Client {
	return defaultClient.SetAutoDecodeAllTypeFunc(fn)
}

// SetAutoDecodeAllTypeFunc set the custmize function that determins the content-type
// whether if should be auto-detected and decode to utf-8
func (c *Client) SetAutoDecodeAllTypeFunc(fn func(contentType string) bool) *Client {
	opt := c.getResponseOptions()
	opt.AutoDecodeContentType = fn
	return c
}

// SetAutoDecodeAllType is a global wrapper methods which delegated
// to the default client's SetAutoDecodeAllType.
func SetAutoDecodeAllType() *Client {
	return defaultClient.SetAutoDecodeAllType()
}

// SetAutoDecodeAllType enables to try auto-detect and decode all content type to utf-8.
func (c *Client) SetAutoDecodeAllType() *Client {
	opt := c.getResponseOptions()
	opt.AutoDecodeContentType = func(contentType string) bool {
		return true
	}
	return c
}

// DisableAutoDecode is a global wrapper methods which delegated
// to the default client's DisableAutoDecode.
func DisableAutoDecode(disable bool) *Client {
	return defaultClient.DisableAutoDecode(disable)
}

// DisableAutoDecode disable auto detect charset and decode to utf-8
func (c *Client) DisableAutoDecode(disable bool) *Client {
	c.getResponseOptions().DisableAutoDecode = disable
	return c
}

// SetUserAgent is a global wrapper methods which delegated
// to the default client's SetUserAgent.
func SetUserAgent(userAgent string) *Client {
	return defaultClient.SetUserAgent(userAgent)
}

// SetUserAgent set the "User-Agent" header for all requests.
func (c *Client) SetUserAgent(userAgent string) *Client {
	return c.SetCommonHeader(hdrUserAgentKey, userAgent)
}

// SetCommonBearerAuthToken is a global wrapper methods which delegated
// to the default client's SetCommonBearerAuthToken.
func SetCommonBearerAuthToken(token string) *Client {
	return defaultClient.SetCommonBearerAuthToken(token)
}

// SetCommonBearerAuthToken set the bearer auth token for all requests.
func (c *Client) SetCommonBearerAuthToken(token string) *Client {
	return c.SetCommonHeader("Authorization", "Bearer "+token)
}

// SetCommonBasicAuth is a global wrapper methods which delegated
// to the default client's SetCommonBasicAuth.
func SetCommonBasicAuth(username, password string) *Client {
	return defaultClient.SetCommonBasicAuth(username, password)
}

// SetCommonBasicAuth set the basic auth for all requests.
func (c *Client) SetCommonBasicAuth(username, password string) *Client {
	c.SetCommonHeader("Authorization", util.BasicAuthHeaderValue(username, password))
	return c
}

// SetCommonHeaders is a global wrapper methods which delegated
// to the default client's SetCommonHeaders.
func SetCommonHeaders(hdrs map[string]string) *Client {
	return defaultClient.SetCommonHeaders(hdrs)
}

// SetCommonHeaders set headers for all requests.
func (c *Client) SetCommonHeaders(hdrs map[string]string) *Client {
	for k, v := range hdrs {
		c.SetCommonHeader(k, v)
	}
	return c
}

// SetCommonHeader is a global wrapper methods which delegated
// to the default client's SetCommonHeader.
func SetCommonHeader(key, value string) *Client {
	return defaultClient.SetCommonHeader(key, value)
}

// SetCommonHeader set a header for all requests.
func (c *Client) SetCommonHeader(key, value string) *Client {
	if c.Headers == nil {
		c.Headers = make(http.Header)
	}
	c.Headers.Set(key, value)
	return c
}

// SetCommonContentType is a global wrapper methods which delegated
// to the default client's SetCommonContentType.
func SetCommonContentType(ct string) *Client {
	return defaultClient.SetCommonContentType(ct)
}

// SetCommonContentType set the `Content-Type` header for all requests.
func (c *Client) SetCommonContentType(ct string) *Client {
	c.SetCommonHeader(hdrContentTypeKey, ct)
	return c
}

// EnableDump is a global wrapper methods which delegated
// to the default client's EnableDump.
func EnableDump(enable bool) *Client {
	return defaultClient.EnableDump(enable)
}

// EnableDump enables dump if set to true, will use a default options if
// not been set before, which dumps all the content of requests and
// responses to stdout.
func (c *Client) EnableDump(enable bool) *Client {
	if !enable {
		c.t.DisableDump()
		return c
	}
	c.enableDump()
	return c
}

// SetDumpOptions is a global wrapper methods which delegated
// to the default client's SetDumpOptions.
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

// SetProxy is a global wrapper methods which delegated
// to the default client's SetProxy.
func SetProxy(proxy func(*http.Request) (*urlpkg.URL, error)) *Client {
	return defaultClient.SetProxy(proxy)
}

// SetProxy set the proxy function.
func (c *Client) SetProxy(proxy func(*http.Request) (*urlpkg.URL, error)) *Client {
	c.t.Proxy = proxy
	return c
}

// OnBeforeRequest is a global wrapper methods which delegated
// to the default client's OnBeforeRequest.
func OnBeforeRequest(m RequestMiddleware) *Client {
	return defaultClient.OnBeforeRequest(m)
}

// OnBeforeRequest add a request middleware which hooks before request sent.
func (c *Client) OnBeforeRequest(m RequestMiddleware) *Client {
	c.udBeforeRequest = append(c.udBeforeRequest, m)
	return c
}

// OnAfterResponse is a global wrapper methods which delegated
// to the default client's OnAfterResponse.
func OnAfterResponse(m ResponseMiddleware) *Client {
	return defaultClient.OnAfterResponse(m)
}

// OnAfterResponse add a response middleware which hooks after response received.
func (c *Client) OnAfterResponse(m ResponseMiddleware) *Client {
	c.afterResponse = append(c.afterResponse, m)
	return c
}

// SetProxyURL is a global wrapper methods which delegated
// to the default client's SetProxyURL.
func SetProxyURL(proxyUrl string) *Client {
	return defaultClient.SetProxyURL(proxyUrl)
}

// SetProxyURL set a proxy from the proxy url.
func (c *Client) SetProxyURL(proxyUrl string) *Client {
	u, err := urlpkg.Parse(proxyUrl)
	if err != nil {
		c.log.Errorf("failed to parse proxy url %s: %v", proxyUrl, err)
		return c
	}
	c.t.Proxy = http.ProxyURL(u)
	return c
}

// EnableTraceAll is a global wrapper methods which delegated
// to the default client's EnableTraceAll.
func EnableTraceAll(enable bool) *Client {
	return defaultClient.EnableTraceAll(enable)
}

// EnableTraceAll enables the trace at client level if set to true.
func (c *Client) EnableTraceAll(enable bool) *Client {
	c.trace = enable
	return c
}

// SetCookieJar is a global wrapper methods which delegated
// to the default client's SetCookieJar.
func SetCookieJar(jar http.CookieJar) *Client {
	return defaultClient.SetCookieJar(jar)
}

// SetCookieJar set the `CookeJar` to the underlying `http.Client`
func (c *Client) SetCookieJar(jar http.CookieJar) *Client {
	c.httpClient.Jar = jar
	return c
}

// SetJsonMarshal is a global wrapper methods which delegated
// to the default client's SetJsonMarshal.
func SetJsonMarshal(fn func(v interface{}) ([]byte, error)) *Client {
	return defaultClient.SetJsonMarshal(fn)
}

// SetJsonMarshal set json marshal function which will be used to marshal request body.
func (c *Client) SetJsonMarshal(fn func(v interface{}) ([]byte, error)) *Client {
	c.jsonMarshal = fn
	return c
}

// SetJsonUnmarshal is a global wrapper methods which delegated
// to the default client's SetJsonUnmarshal.
func SetJsonUnmarshal(fn func(data []byte, v interface{}) error) *Client {
	return defaultClient.SetJsonUnmarshal(fn)
}

// SetJsonUnmarshal set the JSON unmarshal function which will be used to unmarshal response body.
func (c *Client) SetJsonUnmarshal(fn func(data []byte, v interface{}) error) *Client {
	c.jsonUnmarshal = fn
	return c
}

// SetXmlMarshal is a global wrapper methods which delegated
// to the default client's SetXmlMarshal.
func SetXmlMarshal(fn func(v interface{}) ([]byte, error)) *Client {
	return defaultClient.SetXmlMarshal(fn)
}

// SetXmlMarshal set the XML marshal function which will be used to marshal request body.
func (c *Client) SetXmlMarshal(fn func(v interface{}) ([]byte, error)) *Client {
	c.xmlMarshal = fn
	return c
}

// SetXmlUnmarshal is a global wrapper methods which delegated
// to the default client's SetXmlUnmarshal.
func SetXmlUnmarshal(fn func(data []byte, v interface{}) error) *Client {
	return defaultClient.SetXmlUnmarshal(fn)
}

// SetXmlUnmarshal set the XML unmarshal function which will be used to unmarshal response body.
func (c *Client) SetXmlUnmarshal(fn func(data []byte, v interface{}) error) *Client {
	c.xmlUnmarshal = fn
	return c
}

// EnableAllowGetMethodPayload is a global wrapper methods which delegated
// to the default client's EnableAllowGetMethodPayload.
func EnableAllowGetMethodPayload(a bool) *Client {
	return defaultClient.EnableAllowGetMethodPayload(a)
}

// EnableAllowGetMethodPayload allows sending GET method requests with body if set to true.
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
	client := *c.httpClient
	client.Transport = t

	cc := *c
	cc.httpClient = &client
	cc.t = t
	cc.t2 = t2

	cc.Headers = cloneHeaders(c.Headers)
	cc.Cookies = cloneCookies(c.Cookies)
	cc.PathParams = cloneMap(c.PathParams)
	cc.QueryParams = cloneUrlValues(c.QueryParams)
	cc.FormData = cloneUrlValues(c.FormData)
	cc.beforeRequest = cloneRequestMiddleware(c.beforeRequest)
	cc.udBeforeRequest = cloneRequestMiddleware(c.udBeforeRequest)
	cc.afterResponse = cloneResponseMiddleware(c.afterResponse)
	cc.dumpOptions = c.dumpOptions.Clone()

	cc.log = c.log
	cc.jsonUnmarshal = c.jsonUnmarshal
	cc.jsonMarshal = c.jsonMarshal
	cc.xmlMarshal = c.xmlMarshal
	cc.xmlUnmarshal = c.xmlUnmarshal

	return &cc
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
