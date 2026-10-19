package winres

import (
	"bytes"
	"debug/pe"
	"encoding/binary"
	"errors"
	"sort"
	"strconv"
	"strings"
	"unicode/utf16"
)

const (
	rtIcon      = 3
	rtGroupIcon = 14
	rtVersion   = 16
	rtManifest  = 24
	langUS      = 0x0409
	subdir      = 0x80000000
)

var IconSizes = []int{16, 24, 32, 48, 64, 128, 256}

type Icon struct {
	Size int
	PNG  []byte
}

type Version [4]uint16

func ParseVersion(s string) Version {
	var v Version
	for i, part := range strings.SplitN(strings.TrimPrefix(strings.TrimSpace(s), "v"), ".", 4) {
		end := 0
		for end < len(part) && part[end] >= '0' && part[end] <= '9' {
			end++
		}
		n, _ := strconv.Atoi(part[:end])
		v[i] = uint16(min(n, 65535))
	}
	return v
}

func (v Version) String() string {
	return strconv.Itoa(int(v[0])) + "." + strconv.Itoa(int(v[1])) + "." + strconv.Itoa(int(v[2])) + "." + strconv.Itoa(int(v[3]))
}

type Info struct {
	Product     string
	Description string
	Company     string
	Copyright   string
	Original    string
	Internal    string
	VersionText string
	Version     Version
}

type resource struct {
	typ  uint16
	id   uint16
	data []byte
}

const appManifest = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<assembly xmlns="urn:schemas-microsoft-com:asm.v1" manifestVersion="1.0">
  <application xmlns="urn:schemas-microsoft-com:asm.v3">
    <windowsSettings>
      <dpiAware xmlns="http://schemas.microsoft.com/SMI/2005/WindowsSettings">true/pm</dpiAware>
      <dpiAwareness xmlns="http://schemas.microsoft.com/SMI/2016/WindowsSettings">PerMonitorV2</dpiAwareness>
    </windowsSettings>
  </application>
  <compatibility xmlns="urn:schemas-microsoft-com:compatibility.v1">
    <application>
      <supportedOS Id="{8e0f7a12-bfb3-4fe8-b9a5-48fd50a15a9a}"/>
    </application>
  </compatibility>
</assembly>
`

func machine(arch string) (uint16, uint16, error) {
	switch arch {
	case "amd64":
		return 0x8664, 0x0003, nil
	case "386":
		return 0x014c, 0x0007, nil
	case "arm64":
		return 0xaa64, 0x0002, nil
	}
	return 0, 0, errors.New("winres: unsupported architecture " + arch)
}

func Syso(arch string, icons []Icon, info Info, manifest bool) ([]byte, error) {
	mach, rel, err := machine(arch)
	if err != nil {
		return nil, err
	}
	var res []resource
	if len(icons) > 0 {
		for i, ic := range icons {
			res = append(res, resource{typ: rtIcon, id: uint16(i + 1), data: ic.PNG})
		}
		res = append(res, resource{typ: rtGroupIcon, id: 1, data: groupIcon(icons)})
	}
	res = append(res, resource{typ: rtVersion, id: 1, data: versionInfo(info)})
	if manifest {
		res = append(res, resource{typ: rtManifest, id: 1, data: []byte(appManifest)})
	}
	sec, relocs := section(res)
	return coff(mach, rel, arch == "386", sec, relocs), nil
}

func groupIcon(icons []Icon) []byte {
	le := binary.LittleEndian
	b := make([]byte, 6+14*len(icons))
	le.PutUint16(b[2:], 1)
	le.PutUint16(b[4:], uint16(len(icons)))
	for i, ic := range icons {
		e := b[6+14*i:]
		w := byte(ic.Size)
		if ic.Size >= 256 {
			w = 0
		}
		e[0], e[1] = w, w
		le.PutUint16(e[4:], 1)
		le.PutUint16(e[6:], 32)
		le.PutUint32(e[8:], uint32(len(ic.PNG)))
		le.PutUint16(e[12:], uint16(i+1))
	}
	return b
}

type vnode struct {
	key   string
	value []byte
	text  bool
	kids  []vnode
}

func utf16z(s string) []byte {
	u := utf16.Encode([]rune(s))
	b := make([]byte, 2*(len(u)+1))
	for i, c := range u {
		binary.LittleEndian.PutUint16(b[2*i:], c)
	}
	return b
}

func pad4(b *bytes.Buffer) {
	for b.Len()%4 != 0 {
		b.WriteByte(0)
	}
}

func (n vnode) bytes() []byte {
	var b bytes.Buffer
	b.Write(make([]byte, 6))
	b.Write(utf16z(n.key))
	pad4(&b)
	b.Write(n.value)
	for _, k := range n.kids {
		pad4(&b)
		b.Write(k.bytes())
	}
	out := b.Bytes()
	vl, typ := len(n.value), 0
	if n.text {
		vl, typ = len(n.value)/2, 1
	}
	binary.LittleEndian.PutUint16(out[0:], uint16(len(out)))
	binary.LittleEndian.PutUint16(out[2:], uint16(vl))
	binary.LittleEndian.PutUint16(out[4:], uint16(typ))
	return out
}

func fixed(v Version) []byte {
	le := binary.LittleEndian
	b := make([]byte, 52)
	ms := uint32(v[0])<<16 | uint32(v[1])
	ls := uint32(v[2])<<16 | uint32(v[3])
	le.PutUint32(b[0:], 0xFEEF04BD)
	le.PutUint32(b[4:], 0x00010000)
	le.PutUint32(b[8:], ms)
	le.PutUint32(b[12:], ls)
	le.PutUint32(b[16:], ms)
	le.PutUint32(b[20:], ls)
	le.PutUint32(b[24:], 0x3F)
	le.PutUint32(b[32:], 0x00040004)
	le.PutUint32(b[36:], 1)
	return b
}

func versionInfo(info Info) []byte {
	text := info.VersionText
	if text == "" {
		text = info.Version.String()
	}
	pairs := [][2]string{
		{"CompanyName", info.Company}, {"FileDescription", info.Description}, {"FileVersion", text},
		{"InternalName", info.Internal}, {"LegalCopyright", info.Copyright}, {"OriginalFilename", info.Original},
		{"ProductName", info.Product}, {"ProductVersion", text},
	}
	var strs []vnode
	for _, kv := range pairs {
		if kv[1] != "" {
			strs = append(strs, vnode{key: kv[0], value: utf16z(kv[1]), text: true})
		}
	}
	tr := make([]byte, 4)
	binary.LittleEndian.PutUint16(tr[0:], langUS)
	binary.LittleEndian.PutUint16(tr[2:], 0x04B0)
	root := vnode{key: "VS_VERSION_INFO", value: fixed(info.Version), kids: []vnode{
		{key: "StringFileInfo", text: true, kids: []vnode{{key: "040904B0", text: true, kids: strs}}},
		{key: "VarFileInfo", text: true, kids: []vnode{{key: "Translation", value: tr}}},
	}}
	return root.bytes()
}

func align(n, a int) int {
	return (n + a - 1) &^ (a - 1)
}

func section(res []resource) ([]byte, []uint32) {
	le := binary.LittleEndian
	sort.SliceStable(res, func(i, j int) bool {
		if res[i].typ != res[j].typ {
			return res[i].typ < res[j].typ
		}
		return res[i].id < res[j].id
	})
	var types []uint16
	count := map[uint16]int{}
	for _, r := range res {
		if count[r.typ] == 0 {
			types = append(types, r.typ)
		}
		count[r.typ]++
	}
	off := 16 + 8*len(types)
	typeOff := map[uint16]int{}
	for _, t := range types {
		typeOff[t] = off
		off += 16 + 8*count[t]
	}
	langOff := make([]int, len(res))
	for i := range res {
		langOff[i] = off
		off += 24
	}
	entryOff := make([]int, len(res))
	for i := range res {
		entryOff[i] = off
		off += 16
	}
	dataOff := make([]int, len(res))
	for i, r := range res {
		off = align(off, 8)
		dataOff[i] = off
		off += len(r.data)
	}
	b := make([]byte, align(off, 8))
	le.PutUint16(b[14:], uint16(len(types)))
	for i, t := range types {
		le.PutUint32(b[16+8*i:], uint32(t))
		le.PutUint32(b[20+8*i:], uint32(typeOff[t])|subdir)
		le.PutUint16(b[typeOff[t]+14:], uint16(count[t]))
	}
	next := map[uint16]int{}
	relocs := make([]uint32, len(res))
	for i, r := range res {
		at := typeOff[r.typ] + 16 + 8*next[r.typ]
		next[r.typ]++
		le.PutUint32(b[at:], uint32(r.id))
		le.PutUint32(b[at+4:], uint32(langOff[i])|subdir)
		le.PutUint16(b[langOff[i]+14:], 1)
		le.PutUint32(b[langOff[i]+16:], langUS)
		le.PutUint32(b[langOff[i]+20:], uint32(entryOff[i]))
		le.PutUint32(b[entryOff[i]:], uint32(dataOff[i]))
		le.PutUint32(b[entryOff[i]+4:], uint32(len(r.data)))
		copy(b[dataOff[i]:], r.data)
		relocs[i] = uint32(entryOff[i])
	}
	return b, relocs
}

func coff(mach, relType uint16, i386 bool, sec []byte, relocs []uint32) []byte {
	const fileHdr, secHdr, relSize, symSize = 20, 40, 10, 18
	le := binary.LittleEndian
	raw := fileHdr + secHdr
	relPtr := raw + len(sec)
	symPtr := relPtr + relSize*len(relocs)
	b := make([]byte, symPtr+symSize+4)
	le.PutUint16(b[0:], mach)
	le.PutUint16(b[2:], 1)
	le.PutUint32(b[8:], uint32(symPtr))
	le.PutUint32(b[12:], 1)
	if i386 {
		le.PutUint16(b[18:], 0x0100)
	}
	s := b[fileHdr:]
	copy(s[0:8], ".rsrc")
	le.PutUint32(s[16:], uint32(len(sec)))
	le.PutUint32(s[20:], uint32(raw))
	le.PutUint32(s[24:], uint32(relPtr))
	le.PutUint16(s[32:], uint16(len(relocs)))
	le.PutUint32(s[36:], 0x40000040)
	copy(b[raw:], sec)
	for i, r := range relocs {
		o := relPtr + relSize*i
		le.PutUint32(b[o:], r)
		le.PutUint16(b[o+8:], relType)
	}
	sym := b[symPtr:]
	copy(sym[0:8], ".rsrc")
	le.PutUint16(sym[12:], 1)
	sym[16] = 3
	le.PutUint32(b[symPtr+symSize:], 4)
	return b
}

func Types(data []byte) ([]uint16, error) {
	f, err := pe.NewFile(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	s := f.Section(".rsrc")
	if s == nil {
		return nil, errors.New("winres: no .rsrc section")
	}
	raw, err := s.Data()
	if err != nil {
		return nil, err
	}
	if len(raw) < 16 {
		return nil, errors.New("winres: .rsrc is truncated")
	}
	n := int(binary.LittleEndian.Uint16(raw[12:])) + int(binary.LittleEndian.Uint16(raw[14:]))
	var out []uint16
	for i := 0; i < n && 16+8*i+8 <= len(raw); i++ {
		if id := binary.LittleEndian.Uint32(raw[16+8*i:]); id&subdir == 0 {
			out = append(out, uint16(id))
		}
	}
	return out, nil
}
