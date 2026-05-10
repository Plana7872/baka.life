package diff

import (
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/NyanLoli-Network/baka.life/registryctl/provider"
)

type Action string

const (
	Create Action = "create"
	Update Action = "update"
	Delete Action = "delete"
)

type Change struct {
	Action  Action
	Key     string
	Current *provider.Record
	Desired *provider.Record
}

func Generate(current, desired []provider.Record) []Change {
	currentByKey := map[string]provider.Record{}
	desiredByKey := map[string]provider.Record{}

	for _, record := range current {
		currentByKey[record.Key()] = normalize(record)
	}
	for _, record := range desired {
		desiredByKey[record.Key()] = normalize(record)
	}

	keys := map[string]struct{}{}
	for key := range currentByKey {
		keys[key] = struct{}{}
	}
	for key := range desiredByKey {
		keys[key] = struct{}{}
	}

	sortedKeys := make([]string, 0, len(keys))
	for key := range keys {
		sortedKeys = append(sortedKeys, key)
	}
	sort.Strings(sortedKeys)

	changes := make([]Change, 0)
	for _, key := range sortedKeys {
		currentRecord, hasCurrent := currentByKey[key]
		desiredRecord, hasDesired := desiredByKey[key]
		switch {
		case !hasCurrent && hasDesired:
			desiredCopy := desiredRecord
			changes = append(changes, Change{Action: Create, Key: key, Desired: &desiredCopy})
		case hasCurrent && !hasDesired:
			currentCopy := currentRecord
			changes = append(changes, Change{Action: Delete, Key: key, Current: &currentCopy})
		case hasCurrent && hasDesired && !equalRecords(currentRecord, desiredRecord):
			currentCopy := currentRecord
			desiredCopy := desiredRecord
			desiredCopy.ID = currentRecord.ID
			changes = append(changes, Change{Action: Update, Key: key, Current: &currentCopy, Desired: &desiredCopy})
		}
	}

	return changes
}

func FormatChange(change Change) string {
	switch change.Action {
	case Create:
		return fmt.Sprintf("+ create %s %s -> %s", change.Desired.Type, change.Desired.Name, change.Desired.DisplayContent())
	case Update:
		return fmt.Sprintf("~ update %s %s", change.Desired.Type, change.Desired.Name)
	case Delete:
		return fmt.Sprintf("- delete %s %s", change.Current.Type, change.Current.Name)
	default:
		return fmt.Sprintf("? %s", change.Key)
	}
}

func FormatChanges(changes []Change) string {
	lines := make([]string, 0, len(changes))
	for _, change := range changes {
		lines = append(lines, FormatChange(change))
	}
	return strings.Join(lines, "\n")
}

func equalRecords(a, b provider.Record) bool {
	a = normalize(a)
	b = normalize(b)
	a.ID = ""
	b.ID = ""
	return reflect.DeepEqual(a, b)
}

func normalize(record provider.Record) provider.Record {
	record.Type = strings.ToUpper(record.Type)
	record.Name = strings.ToLower(strings.TrimSuffix(record.Name, "."))
	return record
}
