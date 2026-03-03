package exhttp

import (
	"crypto/tls"
	"crypto/x509"
	"log"
	"net/http"
	"net/url"
	"os"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// Config global http DefaultClient
func SetDefaultHTTPClient(httpclient http.Client) {
	singletonHttpclient := http.DefaultClient
	*singletonHttpclient = httpclient
}

func GetDefaultHTTPClient() *http.Client {
	return http.DefaultClient
}

// NewHTTPClient ..if timeout == nil -> use default value.
func NewHTTPClient() *http.Client {
	defaultTransport, _ := http.DefaultTransport.(*http.Transport)
	newTransport := defaultTransport.Clone()
	return &http.Client{
		Transport: newTransport,
	}
}

func setTLS(
	certFile *string,
	keyFile *string,
	caFile *string,
	insecureSkipVerify bool,
) *tls.Config {
	if certFile == nil || keyFile == nil || caFile == nil {
		return nil
	}
	// Load client cert
	cert, err := tls.LoadX509KeyPair(*certFile, *keyFile)
	if err != nil {
		log.Fatal(err)
	}

	// Load CA cert
	caCert, err := os.ReadFile(*caFile)
	if err != nil {
		log.Fatal(err)
	}
	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(caCert)

	// Setup HTTPS client
	tlsConfig := &tls.Config{
		//nolint:gosec // paypay using private SSL
		InsecureSkipVerify: insecureSkipVerify,
		MinVersion:         tls.VersionTLS12,
		Certificates:       []tls.Certificate{cert},
		RootCAs:            caCertPool,
	}
	return tlsConfig
}

// NewHTTPWithCfg ..if timeout == nil -> use default value (60s timeout).
func NewHTTPWithCfg(
	timeout *time.Duration,
	proxy *url.URL,
	certFile *string,
	keyFile *string,
	caFile *string,
	insecureSkipVerify bool,
) *http.Client {
	defaultTransport, _ := http.DefaultTransport.(*http.Transport)
	transport := defaultTransport.Clone()

	if proxy != nil {
		transport.Proxy = http.ProxyURL(proxy)
	}
	tlsConfig := setTLS(certFile, keyFile, caFile, insecureSkipVerify)
	if tlsConfig != nil {
		transport.TLSClientConfig = tlsConfig
	}
	if timeout == nil {
		defaultTimeout := time.Minute
		timeout = &defaultTimeout
	}

	return &http.Client{
		Transport: transport,
		Timeout:   *timeout,
	}
}

// WrapOtelPropagator ..
func WrapOtelPropagator(
	baseClient *http.Client,
) *http.Client {
	if baseClient == nil {
		baseClient = NewHTTPClient()
	}
	baseClient.Transport = otelhttp.NewTransport(baseClient.Transport)
	return baseClient
}

// WrapLog ..
func WrapLog(baseClient *http.Client, loggingCf LoggingConfig) *http.Client {
	if baseClient == nil {
		baseClient = NewHTTPClient()
	}

	baseClient.Transport = NewLoggingRoundTripper(baseClient.Transport, loggingCf)
	return baseClient
}
