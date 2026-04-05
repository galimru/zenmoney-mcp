package runtime

import (
	"fmt"
	"strings"

	"github.com/nemirlev/zenmoney-go-sdk/v2/models"
)

// LookupMaps resolves IDs to human-readable names for tool responses.
type LookupMaps struct {
	Accounts           map[string]string
	Tags               map[string]string
	Instruments        map[int]string
	AccountInstruments map[string]int
}

// BuildLookupMaps constructs LookupMaps from a sync response.
func BuildLookupMaps(resp models.Response) LookupMaps {
	m := LookupMaps{
		Accounts:           make(map[string]string, len(resp.Account)),
		Tags:               make(map[string]string, len(resp.Tag)),
		Instruments:        make(map[int]string, len(resp.Instrument)),
		AccountInstruments: make(map[string]int, len(resp.Account)),
	}
	for _, instr := range resp.Instrument {
		sym := instr.Symbol
		if sym == "" {
			sym = instr.ShortTitle
		}
		m.Instruments[instr.ID] = sym
	}
	for _, acc := range resp.Account {
		m.Accounts[acc.ID] = acc.Title
		if acc.Instrument != nil {
			m.AccountInstruments[acc.ID] = int(*acc.Instrument)
		}
	}
	for _, tag := range resp.Tag {
		m.Tags[tag.ID] = tag.Title
	}
	return m
}

func (m LookupMaps) AccountName(id string) string {
	if name, ok := m.Accounts[id]; ok {
		return name
	}
	return id
}

func (m LookupMaps) TagNames(ids []string) []string {
	names := make([]string, len(ids))
	for i, id := range ids {
		if name, ok := m.Tags[id]; ok {
			names[i] = name
		} else {
			names[i] = id
		}
	}
	return names
}

func (m LookupMaps) InstrumentSymbol(id int) string {
	if sym, ok := m.Instruments[id]; ok {
		return sym
	}
	return fmt.Sprintf("%d", id)
}

func (m LookupMaps) AccountInstrument(accountID string) (int, bool) {
	id, ok := m.AccountInstruments[accountID]
	return id, ok
}

// ResolveTagRef resolves a tag ID or case-insensitive title to a tag ID.
func (m LookupMaps) ResolveTagRef(ref string) (string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", fmt.Errorf("category is required")
	}
	if _, ok := m.Tags[ref]; ok {
		return ref, nil
	}
	for id, title := range m.Tags {
		if strings.EqualFold(title, ref) {
			return id, nil
		}
	}
	return "", fmt.Errorf("category %q not found", ref)
}
