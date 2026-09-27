package main

import (
	"encoding/binary"
	"math"
	"time"
)

// buildGammaICC returns an ICC v4 display profile with sRGB colorimetry and a VCGT
// (video card gamma table) tag holding the gamma curve. With sRGB colorimetry the
// compositor does no colour conversion of its own, so the only visible effect is the VCGT.
func buildGammaICC(gamma float64) []byte {
	be := binary.BigEndian
	s15 := func(v float64) []byte { return be.AppendUint32(nil, uint32(int32(math.Round(v*65536)))) }
	xyz := func(x, y, z float64) []byte {
		b := append([]byte("XYZ "), 0, 0, 0, 0)
		return append(append(append(b, s15(x)...), s15(y)...), s15(z)...)
	}
	mluc := func(text string) []byte {
		b := append([]byte("mluc"), 0, 0, 0, 0)
		b = be.AppendUint32(b, 1)  // record count
		b = be.AppendUint32(b, 12) // record size
		b = append(b, "enUS"...)
		b = be.AppendUint32(b, uint32(len(text)*2))
		b = be.AppendUint32(b, 28) // offset of the string from the tag start
		for _, r := range text {
			b = be.AppendUint16(b, uint16(r))
		}
		return b
	}

	// sRGB transfer function as a parametric curve (type 3).
	trc := append([]byte("para"), 0, 0, 0, 0)
	trc = be.AppendUint16(trc, 3)
	trc = append(trc, 0, 0)
	for _, v := range []float64{2.4, 1 / 1.055, 0.055 / 1.055, 1 / 12.92, 0.04045} {
		trc = append(trc, s15(v)...)
	}

	// Bradford adaptation from D65 to the D50 profile connection space.
	chad := append([]byte("sf32"), 0, 0, 0, 0)
	for _, v := range []float64{
		1.0478112, 0.0228866, -0.0501270,
		0.0295424, 0.9904844, -0.0170491,
		-0.0092345, 0.0150436, 0.7521316,
	} {
		chad = append(chad, s15(v)...)
	}

	const vcgtEntries = 256
	vcgt := append([]byte("vcgt"), 0, 0, 0, 0)
	vcgt = be.AppendUint32(vcgt, 0) // table type
	vcgt = be.AppendUint16(vcgt, 3) // channels
	vcgt = be.AppendUint16(vcgt, vcgtEntries)
	vcgt = be.AppendUint16(vcgt, 2) // bytes per entry
	ramp := buildRamp(vcgtEntries, gamma)
	for range 3 {
		for _, v := range ramp {
			vcgt = be.AppendUint16(vcgt, v)
		}
	}

	type tag struct {
		sig  string
		data []byte
	}
	tags := []tag{
		{"desc", mluc("osu-gamma-controller")},
		{"cprt", mluc("No copyright, use freely")},
		{"wtpt", xyz(0.9642, 1.0, 0.8249)},
		{"chad", chad},
		{"rXYZ", xyz(0.4360747, 0.2225045, 0.0139322)},
		{"gXYZ", xyz(0.3850649, 0.7168786, 0.0971045)},
		{"bXYZ", xyz(0.1430804, 0.0606169, 0.7141733)},
		{"rTRC", trc},
		{"gTRC", trc},
		{"bTRC", trc},
		{"vcgt", vcgt},
	}

	pad4 := func(n int) int { return (n + 3) &^ 3 }
	offset := 128 + 4 + 12*len(tags)
	table := be.AppendUint32(nil, uint32(len(tags)))
	var data []byte
	for _, t := range tags {
		table = append(table, t.sig...)
		table = be.AppendUint32(table, uint32(offset+len(data)))
		table = be.AppendUint32(table, uint32(len(t.data)))
		data = append(data, t.data...)
		data = append(data, make([]byte, pad4(len(t.data))-len(t.data))...)
	}

	header := make([]byte, 128)
	be.PutUint32(header[0:], uint32(offset+len(data)))
	be.PutUint32(header[8:], 0x04300000) // version 4.3
	copy(header[12:], "mntr")
	copy(header[16:], "RGB ")
	copy(header[20:], "XYZ ")
	now := time.Now().UTC()
	for i, v := range []int{now.Year(), int(now.Month()), now.Day(), now.Hour(), now.Minute(), now.Second()} {
		be.PutUint16(header[24+2*i:], uint16(v))
	}
	copy(header[36:], "acsp")
	copy(header[68:], append(append(s15(0.9642), s15(1.0)...), s15(0.8249)...)) // PCS illuminant (D50)

	return append(append(header, table...), data...)
}
