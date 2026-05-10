package sync

import (
	"context"

	registrydiff "github.com/NyanLoli-Network/baka.life/registryctl/diff"
	"github.com/NyanLoli-Network/baka.life/registryctl/provider"
)

func Apply(ctx context.Context, dnsProvider provider.DNSProvider, desired []provider.Record) ([]registrydiff.Change, error) {
	current, err := dnsProvider.ListRecords(ctx)
	if err != nil {
		return nil, err
	}

	changes := registrydiff.Generate(current, desired)
	for _, change := range changes {
		switch change.Action {
		case registrydiff.Create:
			if _, err := dnsProvider.CreateRecord(ctx, *change.Desired); err != nil {
				return changes, err
			}
		case registrydiff.Update:
			if _, err := dnsProvider.UpdateRecord(ctx, *change.Desired); err != nil {
				return changes, err
			}
		case registrydiff.Delete:
			if err := dnsProvider.DeleteRecord(ctx, *change.Current); err != nil {
				return changes, err
			}
		}
	}

	return changes, nil
}
