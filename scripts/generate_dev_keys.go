// Command generate_dev_keys creates a local RSA key pair without external dependencies.
package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	output := flag.String("output", "secrets/dev", "directory for generated PEM files")
	flag.Parse()
	if err := generate(*output); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "generate development keys: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Claves RSA de desarrollo creadas en %s\n", *output)
}

func generate(output string) error {
	if err := os.MkdirAll(output, 0o700); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}
	privateKey, err := rsa.GenerateKey(rand.Reader, 3072)
	if err != nil {
		return fmt.Errorf("generate RSA key: %w", err)
	}
	privateBytes, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		return fmt.Errorf("encode private key: %w", err)
	}
	publicBytes, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	if err != nil {
		return fmt.Errorf("encode public key: %w", err)
	}
	if err := writePEM(filepath.Join(output, "jwt_private.pem"), "PRIVATE KEY", privateBytes); err != nil {
		return err
	}
	if err := writePEM(filepath.Join(output, "jwt_public.pem"), "PUBLIC KEY", publicBytes); err != nil {
		return err
	}
	return nil
}

func writePEM(path, blockType string, data []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("open %s: %w", blockType, err)
	}
	if err := pem.Encode(file, &pem.Block{Type: blockType, Bytes: data}); err != nil {
		_ = file.Close()
		return fmt.Errorf("write %s: %w", blockType, err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close %s: %w", blockType, err)
	}
	return nil
}
