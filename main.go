package main

import (
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"unicode"
)

const (
	OpenProtocol int    = 0x3FFF
	Host         string = "localhost"
	Port         string = "9998"
	Type         string = "http"
	HubName      string = "HUB01"
)

type cmd byte

const (
	WHOISHERE cmd = 0x01
	IAMHERE   cmd = 0x02
	GETSTATUS cmd = 0x03
	STATUS    cmd = 0x04
	SETSTATUS cmd = 0x05
	TICK      cmd = 0x06
)

type devType byte

const (
	SmartHub  devType = 0x01
	EnvSensor devType = 0x02
	Switch    devType = 0x03
	Lamp      devType = 0x04
	Socket    devType = 0x05
	Clock     devType = 0x06
)

type CmdBodyBytes interface {
	toBytes() []byte
}

func getConnectiongString(url string) string {
	if url == "" {
		return fmt.Sprintf("%s://%s:%s", Type, Host, Port)
	}
	return fmt.Sprintf(url)
}

func removeSpaces(line string) string {
	var build strings.Builder
	build.Grow(len(line))
	for _, char := range line {
		if !unicode.IsSpace(char) {
			build.WriteRune(char)
		}
	}
	return build.String()
}

type Payload struct {
	Src     int          `json:"src"`
	Dst     int          `json:"dst"`
	Serial  int          `json:"serial"`
	DevType devType      `json:"dev_type"`
	Cmd     cmd          `json:"cmd"`
	CmdBody CmdBodyBytes `json:"cmd_body,omitempty"`
}

func (pld Payload) toBytes() []byte {
	byteArray := make([]byte, 0)
	byteArray = append(byteArray, encodeULEB128(pld.Src)...)
	byteArray = append(byteArray, encodeULEB128(pld.Dst)...)
	byteArray = append(byteArray, encodeULEB128(pld.Serial)...)
	byteArray = append(byteArray, []byte{byte(pld.DevType), byte(pld.Cmd)}...)
	if pld.CmdBody != nil {
		byteArray = append(byteArray, (pld.CmdBody).toBytes()...)
	}
	return byteArray
}

func payloadFromBytes(bytes []byte) *Payload {
	srcULEB, skipFirst := decodeULEB128(bytes)
	dstULEB, skipSecond := decodeULEB128(bytes[skipFirst:])
	serialULEB, skipThird := decodeULEB128(bytes[skipFirst+skipSecond:])
	payload := skipFirst + skipSecond + skipThird
	pld := Payload{
		Src:     srcULEB,
		Dst:     dstULEB,
		Serial:  serialULEB,
		DevType: devType(bytes[payload]),
		Cmd:     cmd(bytes[payload+1]),
		CmdBody: nil,
	}

	cmdBodyBytes := bytes[payload+2:]
	cmdParsed := parsedCMDBody(pld.DevType, pld.Cmd, cmdBodyBytes)
	if cmdParsed != nil {
		pld.CmdBody = cmdParsed
	}
	return &pld
}

type Packet struct {
	Length  byte    `json:"length"`
	Payload Payload `json:"payload"`
	Crc8    byte    `json:"crc8"`
}

func (pact Packet) toBytes() []byte {
	byteArray := make([]byte, 0)
	byteArray = append(byteArray, pact.Length)
	byteArray = append(byteArray, pact.Payload.toBytes()...)
	byteArray = append(byteArray, pact.Crc8)
	return byteArray
}

func packetFromBytes(bytes []byte) (*Packet, int) {
	dataLength := bytes[0]
	data := bytes[1 : dataLength+1]
	crc8 := bytes[dataLength+1]
	crc8cmp := computeCRC8Simple(data)

	if crc8 != crc8cmp {
		log.Fatal("control sum mismatched")
	}
	pld := payloadFromBytes(data)
	pct := Packet{
		Length:  dataLength,
		Payload: *pld,
		Crc8:    crc8,
	}
	return &pct, int(dataLength) + 2
}

type Packets []Packet

func (pcts Packets) toBytes() []byte {
	byteArray := make([]byte, 0)
	for _, pct := range pcts {
		byteArray = append(byteArray, pct.toBytes()...)
	}
	return byteArray
}

func packetsFromBytes(bytes []byte) *Packets {
	length := len(bytes)
	skip := 0
	var pcts Packets
	for skip < length {
		pct, nowSkip := packetFromBytes(bytes[skip:])
		skip += nowSkip
		pcts = append(pcts, *pct)
	}
	return &pcts
}

func encodeULEB128(value int) []byte {
	var res []byte
	for {
		bt := byte(value & 0x7f)
		value >>= 7
		if value != 0 {
			bt |= 0x80
		}
		res = append(res, bt)
		if value == 0 {
			break
		}
	}
	return res
}

func decodeULEB128(bytes []byte) (int, int) {
	res := 0
	shift := 0
	byteParsed := 0
	for _, bt := range bytes {
		byteParsed++
		res |= (int(bt) & 0x7f) << shift
		shift += 7
		if bt&0x80 == 0 {
			break
		}
	}

	return res, byteParsed
}

func computeCRC8Simple(bytes []byte) byte {
	const generator byte = 0x1D
	crc := byte(0)
	for _, currByte := range bytes {
		crc ^= currByte
		for i := 0; i < 8; i++ {
			if (crc & 0x80) != 0 {
				crc = (crc << 1) ^ generator
			} else {
				crc <<= 1
			}
		}
	}
	return crc
}

type Name struct {
	DevName string `json:"dev_name"`
}

func (name Name) toBytes() []byte {
	byteArr := []byte{byte(len(name.DevName))}
	return append(byteArr, []byte(name.DevName)...)
}

type Sensor struct {
	Values []int `json:"values"`
}

func (sen Sensor) toBytes() []byte {
	byteArr := []byte{byte(len(sen.Values))}
	for _, value := range sen.Values {
		byteArr = append(byteArr, encodeULEB128(value)...)
	}
	return byteArr
}

type Sensors struct {
	DevName  string         `json:"dev_name"`
	DevProps EnvSensorProps `json:"dev_props"`
}

func (sen Sensors) toBytes() []byte {
	nameLen := byte(len(sen.DevName))
	name := []byte(sen.DevName)
	sensors := sen.DevProps.Sensors
	triggerLen := len(sen.DevProps.Triggers)
	byteArr := []byte{nameLen}
	byteArr = append(byteArr, name...)
	byteArr = append(byteArr, []byte{sensors, byte(triggerLen)}...)

	for i := 0; i < triggerLen; i++ {
		bytes := []byte{sen.DevProps.Triggers[i].Op}
		bytes = append(bytes, encodeULEB128(sen.DevProps.Triggers[i].Value)...)
		bytes = append(bytes, byte(len(sen.DevProps.Triggers[i].Name)))
		bytes = append(bytes, []byte(sen.DevProps.Triggers[i].Name)...)
		byteArr = append(byteArr, bytes...)
	}

	return byteArr
}

type SwitchDevice struct {
	DevName  string   `json:"dev_name"`
	DevProps DevProps `json:"dev_props"`
}

type DevProps struct {
	DevNames []string `json:"dev_names"`
}

func (swtd SwitchDevice) toBytes() []byte {
	devNameLen := byte(len(swtd.DevName))
	devName := []byte(swtd.DevName)
	devPropsLen := byte(len(swtd.DevProps.DevNames))
	byteArr := []byte{devNameLen}
	byteArr = append(byteArr, devName...)
	byteArr = append(byteArr, devPropsLen)

	for _, devName := range swtd.DevProps.DevNames {
		byteArr = append(byteArr, byte(len(devName)))
		byteArr = append(byteArr, []byte(devName)...)
	}
	return byteArr
}

type Value struct {
	Value byte `json:"value"`
}

func (val Value) toBytes() []byte {
	return []byte{val.Value}
}

type Timestamp struct {
	Timestamp int `json:"timestamp"`
}

func (tmp Timestamp) toBytes() []byte {
	return encodeULEB128(tmp.Timestamp)
}

func findTime(pcts Packets) int {
	for _, pct := range pcts {
		if pct.Payload.DevType == Clock && pct.Payload.Cmd == TICK {
			clockBody := pct.Payload.CmdBody.(Timestamp)
			return clockBody.Timestamp
		}
	}
	return -1
}

type EnvSensorProps struct {
	Sensors  byte      `json:"sensors"`
	Triggers []Trigger `json:"triggers"`
}

type Trigger struct {
	Op    byte   `json:"op"`
	Value int    `json:"value"`
	Name  string `json:"name"`
}

func parsedCMDBody(device devType, cmd cmd, cmdBodyBytes []byte) CmdBodyBytes {
	if (device == Socket || device == SmartHub || device == Lamp || device == Clock) && (cmd == WHOISHERE || cmd == IAMHERE) {
		nameLength := cmdBodyBytes[0]
		name := cmdBodyBytes[1 : nameLength+1]
		return Name{string(name)}
	} else if device == EnvSensor && (cmd == WHOISHERE || cmd == IAMHERE) {
		nameLength := cmdBodyBytes[0]
		name := cmdBodyBytes[1 : nameLength+1]
		sensors := cmdBodyBytes[nameLength+1]
		triggerLength := cmdBodyBytes[nameLength+2]
		triggers := make([]Trigger, triggerLength)
		skip := int(nameLength) + 3

		for i := 0; i < int(triggerLength); i++ {
			op := cmdBodyBytes[skip]
			skip++
			value, skipULEB := decodeULEB128(cmdBodyBytes[skip:])
			nameLen := int(cmdBodyBytes[skip+skipULEB])
			nameDevice := string(cmdBodyBytes[skip+skipULEB+1 : skip+skipULEB+1+nameLen])
			skip += skipULEB + nameLen + 1
			triggers[i] = Trigger{
				Op:    op,
				Value: value,
				Name:  nameDevice,
			}
		}
		return Sensors{
			DevName: string(name),
			DevProps: EnvSensorProps{
				Sensors:  sensors,
				Triggers: triggers,
			},
		}
	} else if (device == Switch || device == EnvSensor || device == Lamp || device == Socket) && cmd == GETSTATUS {
		return nil
	} else if device == EnvSensor && cmd == STATUS {
		valueSize := int(cmdBodyBytes[0])
		values := make([]int, valueSize)
		skip := 1

		for i := 0; i < valueSize; i++ {
			value, skipULEB := decodeULEB128(cmdBodyBytes[skip:])
			values[i] = value
			skip += skipULEB
		}
		return Sensor{Values: values}
	} else if device == Switch && (cmd == WHOISHERE || cmd == IAMHERE) {
		nameLength := cmdBodyBytes[0]
		name := cmdBodyBytes[1 : nameLength+1]
		devNamesLen := int(cmdBodyBytes[nameLength+1])
		devNames := make([]string, devNamesLen)
		skip := nameLength + 2

		for i := 0; i < devNamesLen; i++ {
			nameLen := cmdBodyBytes[skip]
			nameDevice := cmdBodyBytes[skip+1 : skip+1+nameLen]
			devNames[i] = string(nameDevice)
			skip += nameLen + 1
		}
		return SwitchDevice{
			DevName: string(name),
			DevProps: DevProps{
				DevNames: devNames,
			},
		}
	} else if ((device == Switch || device == Lamp || device == Socket) && cmd == STATUS) ||
		((device == Lamp || device == Socket) && cmd == SETSTATUS) {
		value := cmdBodyBytes[0]
		return Value{Value: value}
	} else if device == Clock && cmd == TICK {
		time, _ := decodeULEB128(cmdBodyBytes[:])
		return Timestamp{Timestamp: time}
	}
	return nil
}

func requestServer(url, request string) ([]byte, int, error) {
	client := &http.Client{}
	req := new(http.Request)
	var err error
	if request == "" {
		req, err = http.NewRequest(
			http.MethodPost, getConnectiongString(url), nil,
		)
	} else {
		req, err = http.NewRequest(
			http.MethodPost, getConnectiongString(url), strings.NewReader(request),
		)
	}
	if err != nil {
		return []byte{}, http.StatusBadRequest, err
	}
	responce, err := client.Do(req)
	defer responce.Body.Close()
	if err != nil {
		return []byte{}, http.StatusBadRequest, err
	}
	body, err := io.ReadAll(responce.Body)
	status := responce.StatusCode
	if err != nil {
		return []byte{}, http.StatusBadRequest, err
	}
	return body, status, err
}

type Database struct {
	Address      int       `json:"address"`
	DevName      string    `json:"dev_name"`
	DevType      devType   `json:"dev_type"`
	Status       bool      `json:"status"`
	IsPresent    bool      `json:"is_present"`
	ConnDevs     []string  `json:"conn_devs"`
	SensorValues []int     `json:"sensor_values"`
	Sensors      byte      `json:"sensors"`
	Triggers     []Trigger `json:"triggers"`
	Time         int       `json:"time"`
}

func setState(pcts *Packets, database map[int]*Database, devices []string, state byte, src int, serial *int) {
	for _, item := range database {
		name := item.DevName
		for _, dev := range devices {
			if name == dev {
				var cmdBody CmdBodyBytes = Value{Value: state}
				newPacket := Packet{
					Length: 0,
					Payload: Payload{
						Src:     src,
						Dst:     item.Address,
						Serial:  *serial,
						DevType: item.DevType,
						Cmd:     SETSTATUS,
						CmdBody: cmdBody,
					},
					Crc8: 0,
				}
				*serial++
				newPacket.Crc8 = computeCRC8Simple(newPacket.Payload.toBytes())
				newPacket.Length = byte(len(newPacket.Payload.toBytes()))
				*pcts = append(*pcts, newPacket)
				break
			}
		}
	}
}

func payloadSwitches(pcts *Packets, database map[int]*Database, src int, serial *int) {
	for _, dev := range database {
		if dev.DevType == Switch && dev.IsPresent {
			newPacket := Packet{
				Length: 0,
				Payload: Payload{
					Src:     src,
					Dst:     dev.Address,
					Serial:  *serial,
					DevType: SmartHub,
					Cmd:     GETSTATUS,
					CmdBody: nil,
				},
				Crc8: 0,
			}
			*serial++
			newPacket.Crc8 = computeCRC8Simple(newPacket.Payload.toBytes())
			newPacket.Length = byte(len(newPacket.Payload.toBytes()))
			*pcts = append(*pcts, newPacket)
		}
	}
}

func handler(database map[int]*Database, requestTime map[int][]int, pcts, tasks *Packets, src int, serial *int) {
	answerTime := findTime(*pcts)
	for _, pct := range *pcts {
		val, ok := database[pct.Payload.Src]
		if ok && !val.IsPresent && pct.Payload.Cmd != WHOISHERE {
			continue
		}
		if pct.Payload.Cmd == IAMHERE {
			devt := pct.Payload.DevType
			adress := pct.Payload.Src
			isAlive := answerTime-requestTime[OpenProtocol][0] <= 300

			if devt == Switch {
				body := pct.Payload.CmdBody.(SwitchDevice)
				database[adress] = &Database{
					Address:      adress,
					DevName:      body.DevName,
					DevType:      pct.Payload.DevType,
					Status:       false,
					IsPresent:    isAlive,
					ConnDevs:     body.DevProps.DevNames,
					SensorValues: nil,
					Sensors:      0,
					Triggers:     nil,
					Time:         answerTime,
				}
			} else if devt == EnvSensor {
				body := pct.Payload.CmdBody.(Sensors)
				database[adress] = &Database{
					Address:      adress,
					DevName:      body.DevName,
					DevType:      pct.Payload.DevType,
					Status:       false,
					IsPresent:    isAlive,
					ConnDevs:     nil,
					SensorValues: nil,
					Sensors:      body.DevProps.Sensors,
					Triggers:     body.DevProps.Triggers,
					Time:         answerTime,
				}
			} else {
				body := pct.Payload.CmdBody.(Name)
				database[adress] = &Database{
					Address:      adress,
					DevName:      body.DevName,
					DevType:      pct.Payload.DevType,
					Status:       false,
					IsPresent:    isAlive,
					ConnDevs:     nil,
					SensorValues: nil,
					Sensors:      0,
					Triggers:     nil,
					Time:         answerTime,
				}
			}
		} else if pct.Payload.Cmd == WHOISHERE {
			var cmdBody CmdBodyBytes = Name{DevName: HubName}
			newPacket := Packet{
				Length: 0,
				Payload: Payload{
					Src:     src,
					Dst:     OpenProtocol,
					Serial:  *serial,
					DevType: SmartHub,
					Cmd:     IAMHERE,
					CmdBody: cmdBody,
				},
				Crc8: 0,
			}
			*serial++
			newPacket.Crc8 = computeCRC8Simple(newPacket.Payload.toBytes())
			newPacket.Length = byte(len(newPacket.Payload.toBytes()))
			*tasks = append(*tasks, newPacket)

			devt := pct.Payload.DevType
			adress := pct.Payload.Src
			if devt == Switch {
				body := pct.Payload.CmdBody.(SwitchDevice)
				database[adress] = &Database{
					Address:      adress,
					DevName:      body.DevName,
					DevType:      pct.Payload.DevType,
					Status:       false,
					IsPresent:    true,
					ConnDevs:     body.DevProps.DevNames,
					SensorValues: nil,
					Sensors:      0,
					Triggers:     nil,
					Time:         answerTime,
				}
			} else if devt == EnvSensor {
				body := pct.Payload.CmdBody.(Sensors)
				database[adress] = &Database{
					Address:      adress,
					DevName:      body.DevName,
					DevType:      pct.Payload.DevType,
					Status:       false,
					IsPresent:    true,
					ConnDevs:     nil,
					SensorValues: nil,
					Sensors:      body.DevProps.Sensors,
					Triggers:     body.DevProps.Triggers,
					Time:         answerTime,
				}
			} else {
				body := pct.Payload.CmdBody.(Name)
				database[adress] = &Database{
					Address:      adress,
					DevName:      body.DevName,
					DevType:      pct.Payload.DevType,
					Status:       false,
					IsPresent:    true,
					ConnDevs:     nil,
					SensorValues: nil,
					Sensors:      0,
					Triggers:     nil,
					Time:         answerTime,
				}
			}
		} else if pct.Payload.Cmd == STATUS {
			if pct.Payload.Src != OpenProtocol {
				if len(requestTime[pct.Payload.Src]) >= 2 {
					requestTime[pct.Payload.Src] = requestTime[pct.Payload.Src][1:]
				} else {
					delete(requestTime, pct.Payload.Src)
				}
			}
			if pct.Payload.DevType == Lamp || pct.Payload.DevType == Socket {
				cbv := pct.Payload.CmdBody.(Value)
				if cbv.Value == 1 {
					database[pct.Payload.Src].Status = true
				} else {
					database[pct.Payload.Src].Status = false
				}
			} else if pct.Payload.DevType == Switch {
				cbv := pct.Payload.CmdBody.(Value)
				if cbv.Value == 1 {
					database[pct.Payload.Src].Status = true
					devNamesTurnOn := database[pct.Payload.Src].ConnDevs
					setState(tasks, database, devNamesTurnOn, 1, src, serial)
				} else {
					database[pct.Payload.Src].Status = false
					devNamesTurnOff := database[pct.Payload.Src].ConnDevs
					setState(tasks, database, devNamesTurnOff, 0, src, serial)
				}
			} else if pct.Payload.DevType == EnvSensor {
				values := pct.Payload.CmdBody.(Sensor).Values
				database[pct.Payload.Src].SensorValues = values
				valuesAll := [4]int{-1, -1, -1, -1}
				envSensor := database[pct.Payload.Src]
				sensorTypeMask := envSensor.Sensors
				idx := 0
				for i := 0; i < 4; i++ {
					if sensorTypeMask&1 == 1 {
						valuesAll[i] = values[idx]
						idx++
					}
					sensorTypeMask = sensorTypeMask >> 1
				}
				triggers := envSensor.Triggers

				for _, trigger := range triggers {
					value := trigger.Value
					device := trigger.Name
					op := trigger.Op

					state := op & 1
					op = op >> 1
					greaterThen := op & 1
					op = op >> 1
					sensorType := op

					if greaterThen == 1 {
						if valuesAll[sensorType] > value {
							setState(tasks, database, []string{device}, state, src, serial)
						}
					} else {
						if valuesAll[sensorType] < value && valuesAll[sensorType] != -1 {
							setState(tasks, database, []string{device}, state, src, serial)
						}
					}
				}
			}
		}
	}
}

func server() {
	args := os.Args[1:]
	if len(args) < 2 {
		os.Exit(99)
	}
	url := args[0]
	hubAddress, err := strconv.ParseInt(args[1], 16, 64)
	if err != nil {
		os.Exit(99)
	}

	database := make(map[int]*Database)
	requestTime := make(map[int][]int)

	serial := 1
	var statusCode, hubTime int
	var requestStr string
	var responceRawBytes, responceRawBytesTrimed, responseBytes []byte

	tasks := Packets{}

	for {
		var cbn Name = Name{DevName: HubName}
		pcts := Packets{
			Packet{
				Length: 0,
				Payload: Payload{
					Src:     int(hubAddress),
					Dst:     OpenProtocol,
					Serial:  serial,
					DevType: SmartHub,
					Cmd:     WHOISHERE,
					CmdBody: cbn,
				},
				Crc8: 0,
			},
		}
		serial++
		pcts[0].Length = byte(len(pcts[0].Payload.toBytes()))
		pcts[0].Crc8 = computeCRC8Simple(pcts[0].Payload.toBytes())
		requestStr = base64.RawURLEncoding.EncodeToString(pcts.toBytes())
		responceRawBytes, statusCode, err = requestServer(url, requestStr)
		if err != nil {
			os.Exit(99)
		}

		if statusCode == http.StatusOK {
			responceRawBytesTrimed = []byte(removeSpaces(string(responceRawBytes)))
			responseBytes, err = base64.RawURLEncoding.DecodeString(string(responceRawBytesTrimed))
			if err != nil {
				continue
			}
			responcePackets := packetsFromBytes(responseBytes)
			hubTime = findTime(*responcePackets)
			requestTime[OpenProtocol] = []int{hubTime}
			handler(database, requestTime, responcePackets, &tasks, int(hubAddress), &serial)
			for _, dev := range database {
				dev.IsPresent = true
			}
			break
		} else if statusCode == http.StatusNoContent {
			os.Exit(0)
		} else {
			os.Exit(99)
		}
	}

	for _, device := range database {
		if device.DevType == EnvSensor {
			getStatusRequest := Packet{
				Length: 0,
				Payload: Payload{
					Src:     int(hubAddress),
					Dst:     device.Address,
					Serial:  serial,
					DevType: SmartHub,
					Cmd:     GETSTATUS,
					CmdBody: nil,
				},
				Crc8: 0,
			}
			serial++
			getStatusRequest.Length = byte(len(getStatusRequest.Payload.toBytes()))
			getStatusRequest.Crc8 = computeCRC8Simple(getStatusRequest.Payload.toBytes())
			tasks = append(tasks, getStatusRequest)
		}
	}

	for statusCode == http.StatusOK {
		payloadSwitches(&tasks, database, int(hubAddress), &serial)
		for _, pct := range tasks {
			curCmd := pct.Payload.Cmd
			curDst := pct.Payload.Dst
			if curCmd == GETSTATUS || curCmd == SETSTATUS {
				requestTime[curDst] = append(requestTime[curDst], hubTime)
			}
		}
		requestStr = base64.RawURLEncoding.EncodeToString(tasks.toBytes())
		tasks = Packets{}

		responceRawBytes, statusCode, err = requestServer(url, requestStr)

		if err != nil {
			continue
		}

		responcePackets := packetsFromBytes(responseBytes)

		hubTime = findTime(*responcePackets)

		for address, time := range requestTime {
			if hubTime-time[0] > 300 {
				if _, ok := database[address]; ok {
					database[address].IsPresent = false
				}
				delete(requestTime, address)
			}
		}
		handler(database, requestTime, responcePackets, &tasks, int(hubAddress), &serial)
	}

	if statusCode == http.StatusNoContent {
		os.Exit(0)
	}
	os.Exit(99)
}

func main() {
	server()
}
