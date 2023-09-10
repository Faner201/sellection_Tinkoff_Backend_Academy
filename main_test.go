package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
)

var tests = []struct {
	tcName  string
	request string
}{
	{"SmartHub, WHOISHERE (1, 1):", "DAH_fwEBAQVIVUIwMeE"},
	{"SmartHub, IAMHERE (1, 2):", "DAH_fwIBAgVIVUIwMak"},
	{"EnvSensor, WHOISHERE (2, 1): ", "OAL_fwMCAQhTRU5TT1IwMQ8EDGQGT1RIRVIxD7AJBk9USEVSMgCsjQYGT1RIRVIzCAAGT1RIRVI03Q"},
	{"EnvSensor, IAMHERE (2, 2): ", "OAL_fwQCAghTRU5TT1IwMQ8EDGQGT1RIRVIxD7AJBk9USEVSMgCsjQYGT1RIRVIzCAAGT1RIRVI09w"},
	{"EnvSensor, GETSTATUS (2, 3): ", "BQECBQIDew"},
	{"EnvSensor, STATUS (2, 4): ", "EQIBBgIEBKUB4AfUjgaMjfILrw"},
	{"Switch, WHOISHERE (3, 1): ", "IgP_fwcDAQhTV0lUQ0gwMQMFREVWMDEFREVWMDIFREVWMDO1"},
	{"Switch, IAMHERE (3, 2): ", "IgP_fwgDAghTV0lUQ0gwMQMFREVWMDEFREVWMDIFREVWMDMo"},
	{"Switch, GETSTATUS (3, 3): ", "BQEDCQMDoA"},
	{"Switch, STATUS (3, 4): ", "BgMBCgMEAac"},
	{"Lamp, WHOISHERE (4, 1): ", "DQT_fwsEAQZMQU1QMDG8"},
	{"Lamp, IAMHERE (4, 2): ", "DQT_fwwEAgZMQU1QMDGU"},
	{"Lamp, GETSTATUS (4, 3): ", "BQEEDQQDqw"},
	{"Lamp, STATUS (4, 4): ", "BgQBDgQEAaw"},
	{"Lamp, SETSTATUS (4, 5): ", "BgEEDwQFAeE"},
	{"Socket, WHOISHERE (5, 1): ", "DwX_fxAFAQhTT0NLRVQwMQ4"},
	{"Socket, IAMHERE (5, 2): ", "DwX_fxEFAghTT0NLRVQwMc0"},
	{"Socket, GETSTATUS (5, 3): ", "BQEFEgUD5A"},
	{"Socket, STATUS (5, 4): ", "BgUBEwUEAQ8"},
	{"Socket, SETSTATUS (5, 5): ", "BgEFFAUFAQc"},
	{"Clock, IAMHERE (6, 2): ", "Dgb_fxUGAgdDTE9DSzAxsw"},
	{"Clock, TICK (6, 6): ", "DAb_fxgGBpabldu2NNM"},
}

func TestCase(t *testing.T) {
	for _, req := range tests {
		input := req.request
		cmd := fmt.Sprintf("echo \"%s\" | smart-home-binary/smarthome-darwin-arm64-0.2.2 -B", input)

		trueAns, err := exec.Command("bash", "-c", cmd).Output()
		fmt.Printf("\n%+v\n", req)
		assert.NoError(t, err)

		reqTrimmed := removeSpaces(input)
		data, err := base64.RawURLEncoding.DecodeString(reqTrimmed)
		assert.NoError(t, err)

		pcts := packetsFromBytes(data)

		a := removeSpaces(string(trueAns))
		a = a[1 : len(a)-1]
		var b string

		for _, pct := range *pcts {
			jsonPacket, err := json.Marshal(pct)
			assert.NoError(t, err)
			b += "," + string(jsonPacket)
		}
		b = b[1:]
		assert.Equal(t, a, b)
		assert.Equal(t, string(data), string(pcts.toBytes()))
	}
}
