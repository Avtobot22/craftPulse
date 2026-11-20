package fixture

import "crypto/tls"

func clientUnsafe() *tls.Config {
	return &tls.Config{
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: true,
	}
}

func clientSafe() *tls.Config {
	return &tls.Config{MinVersion: tls.VersionTLS12}
}
