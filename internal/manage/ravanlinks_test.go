package manage

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestRavanLinksGenerateNewSchemeAndReadLegacyLinks(t *testing.T) {
	raw, err := sampleLink().Encode()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(raw, "ravan://2.") {
		t.Fatal("new setup link has wrong scheme")
	}
	want, err := DecodeShareLink(raw)
	if err != nil {
		t.Fatal(err)
	}
	legacy := strings.Replace(raw, "ravan://", "backpack://", 1)
	v1 := sampleLink()
	v1.V = 1
	encoded, err := json.Marshal(v1)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := DecodeShareLink("backpack://1." + gzipB64(encoded)); err != nil || !reflect.DeepEqual(got, v1) {
		t.Fatalf("legacy format 1 failed: %v", err)
	}
	for _, link := range []string{raw, legacy} {
		pasted := "Setup: '" + link[:len(link)/2] + "\n" + link[len(link)/2:] + "'"
		got, err := DecodeShareLink(FindSetupLink(pasted))
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("setup link failed: %v", err)
		}
	}
	test := ConnTestLink{Host: "203.0.113.7", Coord: 40123, Tok: ctNewSecret()}.Short()
	if !strings.HasPrefix(test, "ravan://t.") {
		t.Fatal("new connection-test link has wrong scheme")
	}
	for _, link := range []string{test, strings.Replace(test, "ravan://", "backpack://", 1)} {
		if !IsConnTestLink("Test: '" + link + "'") {
			t.Fatal("test link was not recognized")
		}
		if _, err := DecodeShareLink(link); err == nil {
			t.Fatal("test link accepted as tunnel setup")
		}
	}
	command := InstallCommand(legacy)
	if !strings.Contains(command, "https://raw.githubusercontent.com/sabialballa/Ravan/main/install.sh") || !strings.Contains(command, "link apply 'ravan://") {
		t.Fatal("installer does not use Ravan source and link")
	}
}
