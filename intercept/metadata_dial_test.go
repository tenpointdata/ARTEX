package intercept

import (
	"context"
	"net"
	"testing"
)

func TestDialRefusingMetadata(t *testing.T) {
	for _, addr := range []string{"169.254.169.254:80", "[::ffff:169.254.169.254]:80", "[64:ff9b::a9fe:a9fe]:80", "[fe80::1]:80", "168.63.129.16:80"} {
		_, err := DialRefusingMetadata(context.Background(), "tcp", addr)
		if code, ok := IsTargetBlocked(err); !ok || code != TargetRuleBlocked {
			t.Errorf("%s: err %v, want %s", addr, err, TargetRuleBlocked)
		}
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	c, err := DialRefusingMetadata(context.Background(), "tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("loopback refused: %v", err)
	}
	c.Close()
}
