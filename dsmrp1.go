package dsmrp1

// Parses dsmrp1 telegram

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"github.com/howeyc/crc16"
	"github.com/tarm/serial"
	"log"
	"reflect"
	"strconv"
	"strings"
)

type Tariff int32

const (
	TariffHigh Tariff = 1
	TariffLow         = 2
)

// We noramalize units to kWh, W, s, m3, A and V
var normalizedUnits map[string]float32 = map[string]float32{
	"kWh": 1,
	"kW":  1000,
	"W":   1,
	"s":   1,
	"m3":  1,
	"A":   1,
	"V":   1,
}

type ElectricityData struct {
	KWh       float32 `obis:"1-0:1.8.2" type:"unit"`
	KWhLow    float32 `obis:"1-0:1.8.1" type:"unit"`
	KWhOut    float32 `obis:"1-0:2.8.2" type:"unit"`
	KWhOutLow float32 `obis:"1-0:2.8.1" type:"unit"`
	Tariff    Tariff  `obis:"0-0:96.14.0" type:"int"`

	W         float32  `obis:"1-0:1.7.0" type:"unit"`
	WOut      float32  `obis:"1-0:2.7.0" type:"unit"`
	Threshold *float32 `obis:"0-0:17.0.0" type:"unit"`
	Switch    *string  `obis:"0-0:96.3.10" type:"id"`

	PowerFailures     int32  `obis:"0-0:96.7.21" type:"int"`
	LongPowerFailures int32  `obis:"0-0:96.7.9" type:"int"`
	PowerFailuresLog  string `obis:"1-0:99.97.0" type:"log"`

	L1VoltageSags   int32    `obis:"1-0:32.32.0" type:"int"`
	L1VoltageSwells int32    `obis:"1-0:32.36.0" type:"int"`
	L1Current       float32  `obis:"1-0:31.7.0" type:"unit"`
	L1Voltage       *float32 `obis:"1-0:32.7.0" type:"unit"`
	L1Power         float32  `obis:"1-0:21.7.0" type:"unit"`
	L1PowerOut      float32  `obis:"1-0:22.7.0" type:"unit"`
}

type MultiphaseElectricityData struct {
	L2VoltageSags   int32    `obis:"1-0:52.32.0" type:"int"`
	L2VoltageSwells int32    `obis:"1-0:52.36.0" type:"int"`
	L2Current       float32  `obis:"1-0:51.7.0" type:"unit"`
	L2Voltage       *float32 `obis:"1-0:52.7.0" type:"unit"`
	L2Power         float32  `obis:"1-0:41.7.0" type:"unit"`
	L2PowerOut      float32  `obis:"1-0:42.7.0" type:"unit"`
	L3VoltageSags   int32    `obis:"1-0:72.32.0" type:"int"`
	L3VoltageSwells int32    `obis:"1-0:72.36.0" type:"int"`
	L3Current       float32  `obis:"1-0:71.7.0" type:"unit"`
	L3Voltage       *float32 `obis:"1-0:72.7.0" type:"unit"`
	L3Power         float32  `obis:"1-0:61.7.0" type:"unit"`
	L3PowerOut      float32  `obis:"1-0:62.7.0" type:"unit"`
}

type GasData struct {
	Type       string    `obis:"0-1:24.1.0" type:"id"`
	Id         string    `obis:"0-1:96.1.0" type:"id"`
	Switch     *string   `obis:"0-1:24.4.0" type:"id"`
	LastRecord GasRecord `obis:"0-1:24.2.1" type:"gasrecord"`
}

type GasRecord struct {
	TimeStamp string
	Value     float32
}

type Telegram struct {
	HeaderMarker string
	HeaderId     string

	Electricity           *ElectricityData
	MultiphaseElectricity *MultiphaseElectricityData
	Gas                   *GasData

	P1Version string `obis:"1-3:0.2.8" type:"id"`
	TimeStamp string `obis:"0-0:1.0.0" type:"id"`
	ID        string `obis:"0-0:96.1.1" type:"id"`

	MsgNumeric *string `obis:"0-0:96.13.1" type:"id"`
	MsgTxt     *string `obis:"0-0:96.13.0" type:"id"`

	Other map[string][]string
}

type Meter struct {
	C       chan *Telegram
	s       *serial.Port
	r       *bufio.Reader
	running bool
}

func crc(data []byte) uint16 {
	return crc16.Update(0xffff, crc16.IBMTable, data) ^ 0xffff
}

func NewMeter(serialDev string) (*Meter, error) {
	var m Meter
	var err error

	m.C = make(chan *Telegram, 1)
	m.s, err = serial.OpenPort(&serial.Config{
		Name:     serialDev,
		Baud:     115200,
		Parity:   serial.ParityNone,
		StopBits: serial.Stop1,
	})
	if err != nil {
		return nil, err
	}

	m.r = bufio.NewReader(m.s)
	m.running = true

	go func() {
		for m.running {
			t, err2 := m.readTelegram()
			if err2 != nil {
				log.Printf("Meter: %v", err2)
				continue
			}
			m.C <- t
		}
		close(m.C)
	}()

	return &m, nil
}

// Parse the lines in a telegram.
func parseLines(rawLines [][]byte) (map[string][]string, error) {
	var lines []string
	var ret map[string][]string

	ret = make(map[string][]string)

	// remove superfluous linebreaks
	for _, rawLine := range rawLines {
		sRawLine := string(rawLine)
		if strings.HasPrefix(sRawLine, "(") {
			lines[len(lines)-1] += sRawLine
			continue
		}
		lines = append(lines, string(sRawLine))
	}

	// parse each line
	for _, line := range lines {
		bits := strings.Split(line, "(")
		obis := bits[0]
		args := []string{}
		for _, arg := range bits[1:] {
			if !strings.HasSuffix(arg, ")") {
				return nil, errors.New(fmt.Sprintf(
					"Malformed argument in line %v", line))
			}
			args = append(args, arg[:len(arg)-1])
		}
		_, alreadyPresent := ret[obis]
		if alreadyPresent {
			return nil, errors.New(fmt.Sprintf(
				"Multiple occurances of OBIS %v", obis))
		}

		ret[obis] = args
	}

	return ret, nil
}

func (m *Meter) readTelegram() (*Telegram, []error) {
	var rawLines [][]byte = [][]byte{}
	var line []byte
	var checkSumBody []byte
	var checkSumLine []byte
	var err error
	var ret Telegram

	// wait for header
	for {
		line, err = m.r.ReadBytes(byte('\n'))
		if err != nil {
			return nil, []error{err}
		}
		if !bytes.HasPrefix(line, []byte("/")) {
			log.Printf("Skipping line %v", line)
			continue
		}
		break
	}

	// parse header
	checkSumBody = line

	if len(line) < 5 {
		return nil, []error{errors.New("Header line too short")}
	}

	ret.HeaderMarker = string(line[:6])
	ret.HeaderId = strings.TrimSpace(string(line[6:]))

	line, err = m.r.ReadBytes(byte('\n'))
	if err != nil {
		return nil, []error{err}
	}
	if strings.TrimSpace(string(line)) != "" {
		return nil, []error{errors.New("Line after header is not blank")}
	}
	checkSumBody = append(checkSumBody, line...)

	// read data
	for {
		line, err = m.r.ReadBytes(byte('\n'))
		if bytes.HasPrefix(line, []byte("!")) {
			checkSumLine = line
			break
		}
		checkSumBody = append(checkSumBody, line...)
		rawLines = append(rawLines, bytes.TrimSpace(line))
	}

	// Check CRC
	crc1 := crc(append(checkSumBody, '!'))
	crc2, err := strconv.ParseInt(strings.TrimSpace(string(checkSumLine[1:])),
		16, 32)
	if err != nil {
		return nil, []error{errors.New(
			fmt.Sprintf("Could not parse checksum: %v", err))}
	}

	if int64(crc1) != crc2 {
		return nil, []error{errors.New("CRC mismatch")}
	}

	// parse the lines
	data, err := parseLines(rawLines)
	if err != nil {
		return nil, []error{err}
	}

	errs := []error{}
	errs = append(errs, fillStruct(&ret, data)...)

	if _, present := data["1-0:1.8.1"]; present {
		var e ElectricityData
		errs = append(errs, fillStruct(&e, data)...)
		ret.Electricity = &e
	}

	if _, present := data["1-0:41.7.0"]; present {
		var e MultiphaseElectricityData
		errs = append(errs, fillStruct(&e, data)...)
		ret.MultiphaseElectricity = &e
	}

	if _, present := data["0-1:24.2.1"]; present {
		var g GasData
		errs = append(errs, fillStruct(&g, data)...)
		ret.Gas = &g
	}

	ret.Other = data

	if len(errs) == 0 {
		errs = nil
	}

	return &ret, errs
}

// Parse and normalize OBIS unit value like "123*A"
func parseUnit(v string) (float32, error) {
	bits := strings.SplitN(v, "*", 2)
	if len(bits) != 2 {
		return 0, errors.New(fmt.Sprintf("not a unit %v", v))
	}
	amount, err := strconv.ParseFloat(bits[0], 32)
	if err != nil {
		return 0, errors.New(fmt.Sprintf("could not parse amount: %s", err))
	}
	factor, ok := normalizedUnits[bits[1]]
	if !ok {
		return 0, errors.New(fmt.Sprintf("unknown unit: %v", v))
	}
	return float32(amount) * factor, nil
}

// Fills the given struct (annotated by "obis" and "type" tags) with
// the values from the the telegram.
func fillStruct(s interface{}, data map[string][]string) []error {
	ret := []error{}
	sv := reflect.Indirect(reflect.ValueOf(s))
	st := sv.Type()
	for i := 0; i < st.NumField(); i++ {
		fieldType := st.Field(i)
		if obis, ok := fieldType.Tag.Lookup("obis"); ok {
			args, ok := data[obis]
			if !ok {
				// Create an error for non-optional (i.e. non pointer) fields.
				if fieldType.Type.Kind() != reflect.Ptr {
					ret = append(ret, errors.New(fmt.Sprintf(
						"Missing data for %s", obis)))
				}
				continue
			}
			delete(data, obis)
			field := sv.FieldByIndex(fieldType.Index)
			if fieldType.Type.Kind() == reflect.Ptr {
				// Handle *type as type
				field.Set(reflect.New(fieldType.Type.Elem()))
				field = reflect.Indirect(field)
			}
			switch typ, _ := fieldType.Tag.Lookup("type"); typ {
			case "id":
				if len(args) != 1 {
					ret = append(ret, errors.New(fmt.Sprintf(
						"%s: wrong number of arguments", obis)))
					continue
				}
				field.SetString(args[0])
			case "int":
				if len(args) != 1 {
					ret = append(ret, errors.New(fmt.Sprintf(
						"%s: wrong number of arguments", obis)))
					continue
				}
				i, err := strconv.Atoi(args[0])
				if err != nil {
					ret = append(ret, errors.New(fmt.Sprintf(
						"%s: could not parse amount: %s", obis, err)))
					continue
				}
				field.SetInt(int64(i))
			case "gasrecord":
				var g GasRecord
				if len(args) != 2 {
					ret = append(ret, errors.New(fmt.Sprintf(
						"%s: wrong number of arguments", obis)))
					continue
				}
				v, err := parseUnit(args[1])
				if err != nil {
					ret = append(ret, errors.New(fmt.Sprintf(
						"%s: value: %s", obis, err)))
					continue
				}
				g.Value = v
				g.TimeStamp = args[0]
				field.Set(reflect.ValueOf(g))
			case "unit":
				if len(args) != 1 {
					ret = append(ret, errors.New(fmt.Sprintf(
						"%s: wrong number of arguments", obis)))
					continue
				}
				v, err := parseUnit(args[0])
				if err != nil {
					ret = append(ret, errors.New(fmt.Sprintf(
						"%s: %s", obis, err)))
					continue
				}
				field.SetFloat(float64(v))
			}
		}
	}
	return ret
}
