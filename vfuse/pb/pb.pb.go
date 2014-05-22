// Code generated by protoc-gen-go.
// source: pb.proto
// DO NOT EDIT!

/*
Package pb is a generated protocol buffer package.

It is generated from these files:
	pb.proto

It has these top-level messages:
	Error
	AttrRequest
	AttrResponse
	Attr
	ReaddirRequest
	ReaddirResponse
	DirEntry
*/
package pb

import proto "code.google.com/p/goprotobuf/proto"
import math "math"

// Reference imports to suppress errors if they are not otherwise used.
var _ = proto.Marshal
var _ = math.Inf

type Error struct {
	NotExist         *bool   `protobuf:"varint,1,opt,name=not_exist" json:"not_exist,omitempty"`
	Other            *string `protobuf:"bytes,2,opt,name=other" json:"other,omitempty"`
	XXX_unrecognized []byte  `json:"-"`
}

func (m *Error) Reset()         { *m = Error{} }
func (m *Error) String() string { return proto.CompactTextString(m) }
func (*Error) ProtoMessage()    {}

func (m *Error) GetNotExist() bool {
	if m != nil && m.NotExist != nil {
		return *m.NotExist
	}
	return false
}

func (m *Error) GetOther() string {
	if m != nil && m.Other != nil {
		return *m.Other
	}
	return ""
}

type AttrRequest struct {
	Name             *string `protobuf:"bytes,1,req,name=name" json:"name,omitempty"`
	XXX_unrecognized []byte  `json:"-"`
}

func (m *AttrRequest) Reset()         { *m = AttrRequest{} }
func (m *AttrRequest) String() string { return proto.CompactTextString(m) }
func (*AttrRequest) ProtoMessage()    {}

func (m *AttrRequest) GetName() string {
	if m != nil && m.Name != nil {
		return *m.Name
	}
	return ""
}

type AttrResponse struct {
	Attr             *Attr  `protobuf:"bytes,1,opt,name=attr" json:"attr,omitempty"`
	Err              *Error `protobuf:"bytes,2,opt,name=err" json:"err,omitempty"`
	XXX_unrecognized []byte `json:"-"`
}

func (m *AttrResponse) Reset()         { *m = AttrResponse{} }
func (m *AttrResponse) String() string { return proto.CompactTextString(m) }
func (*AttrResponse) ProtoMessage()    {}

func (m *AttrResponse) GetAttr() *Attr {
	if m != nil {
		return m.Attr
	}
	return nil
}

func (m *AttrResponse) GetErr() *Error {
	if m != nil {
		return m.Err
	}
	return nil
}

type Attr struct {
	Size             *uint64 `protobuf:"varint,1,opt,name=size" json:"size,omitempty"`
	AtimeSec         *uint64 `protobuf:"varint,2,opt,name=atime_sec" json:"atime_sec,omitempty"`
	AtimeNano        *uint32 `protobuf:"varint,3,opt,name=atime_nano" json:"atime_nano,omitempty"`
	MtimeSec         *uint64 `protobuf:"varint,4,opt,name=mtime_sec" json:"mtime_sec,omitempty"`
	MtimeNano        *uint32 `protobuf:"varint,5,opt,name=mtime_nano" json:"mtime_nano,omitempty"`
	Mode             *uint32 `protobuf:"varint,6,opt,name=mode" json:"mode,omitempty"`
	Nlink            *uint32 `protobuf:"varint,7,opt,name=nlink" json:"nlink,omitempty"`
	XXX_unrecognized []byte  `json:"-"`
}

func (m *Attr) Reset()         { *m = Attr{} }
func (m *Attr) String() string { return proto.CompactTextString(m) }
func (*Attr) ProtoMessage()    {}

func (m *Attr) GetSize() uint64 {
	if m != nil && m.Size != nil {
		return *m.Size
	}
	return 0
}

func (m *Attr) GetAtimeSec() uint64 {
	if m != nil && m.AtimeSec != nil {
		return *m.AtimeSec
	}
	return 0
}

func (m *Attr) GetAtimeNano() uint32 {
	if m != nil && m.AtimeNano != nil {
		return *m.AtimeNano
	}
	return 0
}

func (m *Attr) GetMtimeSec() uint64 {
	if m != nil && m.MtimeSec != nil {
		return *m.MtimeSec
	}
	return 0
}

func (m *Attr) GetMtimeNano() uint32 {
	if m != nil && m.MtimeNano != nil {
		return *m.MtimeNano
	}
	return 0
}

func (m *Attr) GetMode() uint32 {
	if m != nil && m.Mode != nil {
		return *m.Mode
	}
	return 0
}

func (m *Attr) GetNlink() uint32 {
	if m != nil && m.Nlink != nil {
		return *m.Nlink
	}
	return 0
}

type ReaddirRequest struct {
	Name             *string `protobuf:"bytes,1,req,name=name" json:"name,omitempty"`
	XXX_unrecognized []byte  `json:"-"`
}

func (m *ReaddirRequest) Reset()         { *m = ReaddirRequest{} }
func (m *ReaddirRequest) String() string { return proto.CompactTextString(m) }
func (*ReaddirRequest) ProtoMessage()    {}

func (m *ReaddirRequest) GetName() string {
	if m != nil && m.Name != nil {
		return *m.Name
	}
	return ""
}

type ReaddirResponse struct {
	Err              *Error      `protobuf:"bytes,1,opt,name=err" json:"err,omitempty"`
	Entry            []*DirEntry `protobuf:"bytes,2,rep,name=entry" json:"entry,omitempty"`
	XXX_unrecognized []byte      `json:"-"`
}

func (m *ReaddirResponse) Reset()         { *m = ReaddirResponse{} }
func (m *ReaddirResponse) String() string { return proto.CompactTextString(m) }
func (*ReaddirResponse) ProtoMessage()    {}

func (m *ReaddirResponse) GetErr() *Error {
	if m != nil {
		return m.Err
	}
	return nil
}

func (m *ReaddirResponse) GetEntry() []*DirEntry {
	if m != nil {
		return m.Entry
	}
	return nil
}

type DirEntry struct {
	Name             *string `protobuf:"bytes,1,req,name=name" json:"name,omitempty"`
	Mode             *uint32 `protobuf:"varint,2,opt,name=mode" json:"mode,omitempty"`
	XXX_unrecognized []byte  `json:"-"`
}

func (m *DirEntry) Reset()         { *m = DirEntry{} }
func (m *DirEntry) String() string { return proto.CompactTextString(m) }
func (*DirEntry) ProtoMessage()    {}

func (m *DirEntry) GetName() string {
	if m != nil && m.Name != nil {
		return *m.Name
	}
	return ""
}

func (m *DirEntry) GetMode() uint32 {
	if m != nil && m.Mode != nil {
		return *m.Mode
	}
	return 0
}

func init() {
}
