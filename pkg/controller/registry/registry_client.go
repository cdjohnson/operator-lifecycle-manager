//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 -o resolver/fakes/fake_registry_client_interface.go . RegistryClientInterface
package registry

import (
	"context"
	"fmt"
	"io"

	registryapi "github.com/operator-framework/operator-registry/pkg/api"
	"github.com/operator-framework/operator-registry/pkg/client"
	opregistry "github.com/operator-framework/operator-registry/pkg/registry"
)

type ChannelEntryStream interface {
	Recv() (*registryapi.ChannelEntry, error)
}

type RegistryClientInterface interface {
	client.Interface
	FindBundleThatProvides(ctx context.Context, group, version, kind, pkgName string) (*registryapi.Bundle, error)
	GetLatestChannelEntriesThatProvide(ctx context.Context, group, version, kind string) (*ChannelEntryIterator, error)
}

type ChannelEntryIterator struct {
	stream ChannelEntryStream
	error  error
}

func NewChannelEntryIterator(stream ChannelEntryStream) *ChannelEntryIterator {
	return &ChannelEntryIterator{stream: stream}
}

func (ceit *ChannelEntryIterator) Next() *registryapi.ChannelEntry {
	if ceit.error != nil {
		return nil
	}
	next, err := ceit.stream.Recv()
	if err == io.EOF {
		return nil
	}
	if err != nil {
		ceit.error = err
	}
	return next
}

func (ceit *ChannelEntryIterator) Error() error {
	return ceit.error
}

type RegistryClient struct {
	client.Interface
	Client *client.Client
}

func NewRegistryClient(client *client.Client) *RegistryClient {
	return &RegistryClient{client, client}
}

var _ RegistryClientInterface = &RegistryClient{}
var _ client.Interface = &RegistryClient{}

// GetLatestChannelEntriesThatProvide uses registry client to get a list of
// latest channel entries that provide the requested API (via an iterator)
func (rc *RegistryClient) GetLatestChannelEntriesThatProvide(ctx context.Context, group, version, kind string) (*ChannelEntryIterator, error) {
	stream, err := rc.Client.Registry.GetLatestChannelEntriesThatProvide(ctx, &registryapi.GetLatestProvidersRequest{Group: group, Version: version, Kind: kind})
	if err != nil {
		return nil, err
	}
	return NewChannelEntryIterator(stream), nil
}

// FindBundleThatProvides returns a bundle that provides the request API and
// doesn't belong to the provided package
func (rc *RegistryClient) FindBundleThatProvides(ctx context.Context, group, version, kind, pkgName string) (*registryapi.Bundle, error) {
	it, err := rc.GetLatestChannelEntriesThatProvide(ctx, group, version, kind)
	if err != nil {
		return nil, err
	}
	entry := rc.filterChannelEntries(it, pkgName)
	if entry == nil {
		return nil, fmt.Errorf("Unable to find a channel entry which doesn't belong to package %s", pkgName)
	}
	bundle, err := rc.Client.Registry.GetBundle(ctx, &registryapi.GetBundleRequest{PkgName: entry.PackageName, ChannelName: entry.ChannelName, CsvName: entry.BundleName})
	if err != nil {
		return nil, err
	}
	return bundle, nil
}

// FilterChannelEntries filters out a channel entries that provide the requested
// API and come from the same package with original operator and returns the
// first entry on the list
func (rc *RegistryClient) filterChannelEntries(it *ChannelEntryIterator, pkgName string) *opregistry.ChannelEntry {
	var entries []*opregistry.ChannelEntry
	for e := it.Next(); e != nil; e = it.Next() {
		if e.PackageName != pkgName {
			entry := &opregistry.ChannelEntry{
				PackageName: e.PackageName,
				ChannelName: e.ChannelName,
				BundleName:  e.BundleName,
				Replaces:    e.Replaces,
			}
			entries = append(entries, entry)
		}
	}

	if entries != nil {
		return entries[0]
	}
	return nil
}
