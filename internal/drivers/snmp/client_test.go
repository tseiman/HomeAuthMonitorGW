package snmp

import (
	"testing"

	gosnmp "github.com/gosnmp/gosnmp"
)

func TestNewGoSNMPConfiguresV3AuthPrivSHA1DES(t *testing.T) {
	g, err := NewGoSNMP("192.0.2.10", 161, "monitor", "auth-placeholder", "privacy-placeholder", 5)
	if err != nil {
		t.Fatal(err)
	}
	if g.Version != gosnmp.Version3 || g.MsgFlags != gosnmp.AuthPriv {
		t.Fatalf("version/security wrong: %+v", g)
	}
	sp, ok := g.SecurityParameters.(*gosnmp.UsmSecurityParameters)
	if !ok {
		t.Fatalf("security=%T", g.SecurityParameters)
	}
	if sp.AuthenticationProtocol != gosnmp.SHA || sp.PrivacyProtocol != gosnmp.DES {
		t.Fatalf("protocols=%+v", sp)
	}
}
