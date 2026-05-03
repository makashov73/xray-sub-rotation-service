package tls

import "crypto/tls"

// LoadAndVerify loads and verifies TLS cert/key files.
func LoadAndVerify(certFile, keyFile string) error {
	_, err := tls.LoadX509KeyPair(certFile, keyFile)
	return err
}
