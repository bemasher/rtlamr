package scmplus

import (
	"encoding/json"
	"encoding/xml"
	"strings"
	"testing"
)

// TestFieldNames pins the JSON and XML output field names so downstream
// consumers (MQTT bridges, rtlamr-collect, scripts) don't break.
func TestFieldNames(t *testing.T) {
	m := SCM{EndpointID: 12345678, PacketCRC: 0xBEEF}

	j, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(j), `"PacketCRC":48879`) {
		t.Errorf("JSON missing PacketCRC: %s", j)
	}

	x, err := xml.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(x), `Checksum="48879"`) {
		t.Errorf("XML missing Checksum attribute: %s", x)
	}
}
