// Code generated by protoc-gen-go-grpc. DO NOT EDIT.
// versions:
// - protoc-gen-go-grpc v1.3.0
// - protoc             v3.21.12
// source: proto/interface.proto

package proto

import (
	context "context"
	grpc "google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
)

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
// Requires gRPC-Go v1.32.0 or later.
const _ = grpc.SupportPackageIsVersion7

const (
	RicartAndAgrawala_Request_FullMethodName = "/proto.RicartAndAgrawala/Request"
	RicartAndAgrawala_Reply_FullMethodName   = "/proto.RicartAndAgrawala/Reply"
)

// RicartAndAgrawalaClient is the client API for RicartAndAgrawala service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
type RicartAndAgrawalaClient interface {
	Request(ctx context.Context, in *Info, opts ...grpc.CallOption) (*Empty, error)
	Reply(ctx context.Context, in *Id, opts ...grpc.CallOption) (*Empty, error)
}

type ricartAndAgrawalaClient struct {
	cc grpc.ClientConnInterface
}

func NewRicartAndAgrawalaClient(cc grpc.ClientConnInterface) RicartAndAgrawalaClient {
	return &ricartAndAgrawalaClient{cc}
}

func (c *ricartAndAgrawalaClient) Request(ctx context.Context, in *Info, opts ...grpc.CallOption) (*Empty, error) {
	out := new(Empty)
	err := c.cc.Invoke(ctx, RicartAndAgrawala_Request_FullMethodName, in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *ricartAndAgrawalaClient) Reply(ctx context.Context, in *Id, opts ...grpc.CallOption) (*Empty, error) {
	out := new(Empty)
	err := c.cc.Invoke(ctx, RicartAndAgrawala_Reply_FullMethodName, in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// RicartAndAgrawalaServer is the server API for RicartAndAgrawala service.
// All implementations must embed UnimplementedRicartAndAgrawalaServer
// for forward compatibility
type RicartAndAgrawalaServer interface {
	Request(context.Context, *Info) (*Empty, error)
	Reply(context.Context, *Id) (*Empty, error)
	mustEmbedUnimplementedRicartAndAgrawalaServer()
}

// UnimplementedRicartAndAgrawalaServer must be embedded to have forward compatible implementations.
type UnimplementedRicartAndAgrawalaServer struct {
}

func (UnimplementedRicartAndAgrawalaServer) Request(context.Context, *Info) (*Empty, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Request not implemented")
}
func (UnimplementedRicartAndAgrawalaServer) Reply(context.Context, *Id) (*Empty, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Reply not implemented")
}
func (UnimplementedRicartAndAgrawalaServer) mustEmbedUnimplementedRicartAndAgrawalaServer() {}

// UnsafeRicartAndAgrawalaServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to RicartAndAgrawalaServer will
// result in compilation errors.
type UnsafeRicartAndAgrawalaServer interface {
	mustEmbedUnimplementedRicartAndAgrawalaServer()
}

func RegisterRicartAndAgrawalaServer(s grpc.ServiceRegistrar, srv RicartAndAgrawalaServer) {
	s.RegisterService(&RicartAndAgrawala_ServiceDesc, srv)
}

func _RicartAndAgrawala_Request_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(Info)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(RicartAndAgrawalaServer).Request(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: RicartAndAgrawala_Request_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(RicartAndAgrawalaServer).Request(ctx, req.(*Info))
	}
	return interceptor(ctx, in, info, handler)
}

func _RicartAndAgrawala_Reply_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(Id)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(RicartAndAgrawalaServer).Reply(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: RicartAndAgrawala_Reply_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(RicartAndAgrawalaServer).Reply(ctx, req.(*Id))
	}
	return interceptor(ctx, in, info, handler)
}

// RicartAndAgrawala_ServiceDesc is the grpc.ServiceDesc for RicartAndAgrawala service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var RicartAndAgrawala_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "proto.RicartAndAgrawala",
	HandlerType: (*RicartAndAgrawalaServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "Request",
			Handler:    _RicartAndAgrawala_Request_Handler,
		},
		{
			MethodName: "Reply",
			Handler:    _RicartAndAgrawala_Reply_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "proto/interface.proto",
}