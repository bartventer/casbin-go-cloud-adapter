package adapter

import (
	"context"
	//nolint:gosec // we don't need a secure hash, hence we use md5
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"runtime"
	"strings"
	"time"

	"github.com/casbin/casbin/v2/model"
	"github.com/casbin/casbin/v2/persist"
	"gocloud.dev/docstore"
)

const (
	defaultTimeout time.Duration = 30 * time.Second
)

// CasbinRule represents a rule in Casbin.
type CasbinRule struct {
	PType string `docstore:"ptype"`
	V0    string `docstore:"v0"`
	V1    string `docstore:"v1,omitempty"`
	V2    string `docstore:"v2,omitempty"`
	V3    string `docstore:"v3,omitempty"`
	V4    string `docstore:"v4,omitempty"`
	V5    string `docstore:"v5,omitempty"`
	ID    string `docstore:"id"`
}

// Adapter is the interface for Casbin adapters supporting [batch], [filtered] and [auto-save] features.
//
// [batch]: https://pkg.go.dev/github.com/casbin/casbin/v2/persist#BatchAdapter
// [filtered]: https://pkg.go.dev/github.com/casbin/casbin/v2/persist#FilteredAdapter
// [auto-save]: https://pkg.go.dev/github.com/casbin/casbin/v2/persist#UpdatableAdapter
type Adapter interface {
	// BatchAdapter is the interface for Casbin adapters with multiple add and remove policy functions.
	persist.BatchAdapter
	// FilteredAdapter is the interface for Casbin adapters with policy filtering feature.
	persist.FilteredAdapter
	// UpdatableAdapter is the interface for Casbin adapters with auto-save feature.
	persist.UpdatableAdapter
}

var _ Adapter = (*adapter)(nil)

// adapter implements [Adapter].
type adapter struct {
	collection *docstore.Collection
	timeout    time.Duration
	filtered   bool
	config     *Config
}

// finalizer is the destructor for adapter.
func finalizer(a *adapter) {
	a.close()
}

// NewFilteredAdapter is the constructor for FilteredAdapter.
// Casbin will not automatically call LoadPolicy() for a filtered adapter.
func NewFilteredAdapter(ctx context.Context, url string) (*adapter, error) {
	a, err := NewWithOption(ctx, &Config{URL: url})
	if err != nil {
		return nil, err
	}
	a.filtered = true

	return a, nil
}

// Config is the configuration for Adapter.
type Config struct {
	Timeout    time.Duration // the timeout for any operations on the adapter
	IsFiltered bool          // whether the adapter is filtered
	URL        string        // the driver url (e.g. mongodb://localhost:27017)
}

// New is the constructor for Adapter.
func New(ctx context.Context, url string) (*adapter, error) {
	return NewWithOption(ctx, &Config{URL: url})
}

// NewWithOption is the constructor for Adapter with option.
func NewWithOption(ctx context.Context, config *Config) (*adapter, error) {
	if config == nil {
		config = &Config{}
	}
	if config.Timeout == 0 {
		config.Timeout = defaultTimeout
	}

	coll, err := docstore.OpenCollection(ctx, config.URL)
	if err != nil {
		return nil, fmt.Errorf("could not open collection: %v", err)
	}

	a := &adapter{
		collection: coll,
		timeout:    config.Timeout,
		filtered:   config.IsFiltered,
		config:     config,
	}

	// Call the destructor when the object is released.
	runtime.SetFinalizer(a, finalizer)

	return a, nil
}

func (a *adapter) close() {
	if a.collection != nil {
		err := a.collection.Close()
		if err != nil {
			log.Printf("close collection error: %v", err)
		}
		a.collection = nil
	}
}

func loadPolicyLine(line CasbinRule, model model.Model) error {
	p := [...]string{line.PType, line.V0, line.V1, line.V2, line.V3, line.V4, line.V5}

	var lineText string
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] != "" {
			lineText = strings.Join(p[:i+1], ", ")
			break
		}
	}

	return persist.LoadPolicyLine(lineText, model)
}

// LoadPolicy loads policy from database.
func (a *adapter) LoadPolicy(model model.Model) error {
	return a.LoadFilteredPolicy(model, nil)
}

// LoadFilteredPolicy loads matching policy lines from database. If not nil,
// the filter must be a valid MongoDB selector.
func (a *adapter) LoadFilteredPolicy(model model.Model, filter interface{}) error {
	filters := make([]Filter, 0)
	if filter == nil {
		a.filtered = false
	} else {
		a.filtered = true
		switch filterValue := filter.(type) {
		case Filter:
			filters = append(filters, filterValue)
		case *Filter:
			filters = append(filters, *filterValue)
		case []Filter:
			filters = append(filters, filterValue...)
		case *[]Filter:
			filters = append(filters, *filterValue...)
		default:
			return errors.New("invalid filter type")
		}
	}

	ctx, cancel := context.WithTimeout(context.TODO(), a.timeout)
	defer cancel()

	query := a.collection.Query()
	if len(filters) > 0 {
		for _, f := range filters {
			fieldPath := docstore.FieldPath(strings.Join(f.FieldPath, ".")) // dot seperated path (e.g. "field.subfield")
			if f.Op == "" {                                                 // default to ==
				f.Op = EqualOp
			}
			query = query.Where(fieldPath, f.Op, f.Value)
		}
	}
	iter := query.Get(ctx)
	defer iter.Stop()
	for {
		var line CasbinRule
		err := iter.Next(ctx, &line)
		if err == io.EOF {
			break
		} else if err != nil {
			return err
		} else {
			err = loadPolicyLine(line, model)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// IsFiltered returns true if the loaded policy has been filtered.
func (a *adapter) IsFiltered() bool {
	return a.filtered
}

// generateID generates an ID for a CasbinRule.
func generateID(line CasbinRule) string {
	data := []byte(fmt.Sprint(line))
	hash := md5.Sum(data) //nolint:gosec // we don't need a secure hash here
	return hex.EncodeToString(hash[:])
}

func savePolicyLine(ptype string, rule []string) CasbinRule {
	line := CasbinRule{
		PType: ptype,
	}

	fields := [...]*string{&line.V0, &line.V1, &line.V2, &line.V3, &line.V4, &line.V5}
	for i := 0; i < len(rule) && i < len(fields); i++ {
		*fields[i] = rule[i]
	}

	// set md5 hash as id
	line.ID = generateID(line)

	return line
}

// SavePolicy saves policy to database.
func (a *adapter) SavePolicy(model model.Model) error {
	if a.filtered {
		return errors.New("cannot save a filtered policy")
	}

	ctx, cancel := context.WithTimeout(context.TODO(), a.timeout)
	defer cancel()
	actionList := a.collection.Actions()
	for _, typ := range [...]string{"p", "g"} {
		if ast, ok := model[typ]; ok {
			for ptype, ast := range ast {
				for _, rule := range ast.Policy {
					line := savePolicyLine(ptype, rule)
					actionList.Put(&line)
				}
			}
		}
	}

	if err := actionList.Do(ctx); err != nil {
		return err
	}

	return nil
}

// AddPolicy adds a policy rule to the storage.
func (a *adapter) AddPolicy(sec string, ptype string, rule []string) error {
	line := savePolicyLine(ptype, rule)

	ctx, cancel := context.WithTimeout(context.TODO(), a.timeout)
	defer cancel()

	if err := a.collection.Actions().Put(&line).Do(ctx); err != nil {
		return err
	}

	return nil
}

// AddPolicies adds policy rules to the storage.
func (a *adapter) AddPolicies(sec string, ptype string, rules [][]string) error {
	ctx, cancel := context.WithTimeout(context.TODO(), a.timeout)
	defer cancel()
	actionList := a.collection.Actions()
	for _, rule := range rules {
		line := savePolicyLine(ptype, rule)
		actionList.Put(&line)
	}
	if err := actionList.Do(ctx); err != nil {
		return err
	}

	return nil
}

// RemovePolicies removes policy rules from the storage.
func (a *adapter) RemovePolicies(sec string, ptype string, rules [][]string) error {
	ctx, cancel := context.WithTimeout(context.TODO(), a.timeout)
	defer cancel()
	actionList := a.collection.Actions()
	for _, rule := range rules {
		line := savePolicyLine(ptype, rule)
		actionList.Delete(&line)
	}
	if err := actionList.Do(ctx); err != nil {
		return err
	}

	return nil
}

// RemovePolicy removes a policy rule from the storage.
func (a *adapter) RemovePolicy(sec string, ptype string, rule []string) error {
	line := savePolicyLine(ptype, rule)

	ctx, cancel := context.WithTimeout(context.TODO(), a.timeout)
	defer cancel()
	if err := a.collection.Delete(ctx, &line); err != nil {
		return err
	}

	return nil
}

// RemoveFilteredPolicy removes policy rules that match the filter from the storage.
func (a *adapter) RemoveFilteredPolicy(sec string, ptype string, fieldIndex int, fieldValues ...string) error {
	query := a.collection.Query().Where(docstore.FieldPath("ptype"), EqualOp, ptype)

	for i := 0; i <= 5; i++ { // max 6 filters (v0-v5)
		query = a.addFiltersToQuery(query, i, fieldIndex, fieldValues...)
	}

	ctx, cancel := context.WithTimeout(context.TODO(), a.timeout)
	defer cancel()
	iter := query.Get(ctx)
	defer iter.Stop()

	// delete the document
	actionList := a.collection.Actions()
	for {
		got := new(CasbinRule)
		err := iter.Next(ctx, got)
		if err == io.EOF {
			break
		} else if err != nil {
			return err
		} else {
			actionList.Delete(got)
		}
	}

	if err := actionList.Do(ctx); err != nil {
		return err
	}

	return nil
}

// UpdatePolicy updates a policy rule from storage.
// This is part of the Auto-Save feature.
func (a *adapter) UpdatePolicy(sec string, ptype string, oldRule, newPolicy []string) error {
	oldLine := savePolicyLine(ptype, oldRule)
	newLine := savePolicyLine(ptype, newPolicy)

	ctx, cancel := context.WithTimeout(context.TODO(), a.timeout)
	defer cancel()
	if err := a.collection.Actions().Delete(&oldLine).Put(&newLine).Do(ctx); err != nil {
		return err
	}

	return nil
}

// UpdatePolicies updates some policy rules to storage, like db, redis.
func (a *adapter) UpdatePolicies(sec string, ptype string, oldRules, newRules [][]string) error {
	ctx, cancel := context.WithTimeout(context.TODO(), a.timeout)
	defer cancel()
	for i := range oldRules {
		oldLine := savePolicyLine(ptype, oldRules[i])
		newLine := savePolicyLine(ptype, newRules[i])

		// delete and put
		if err := a.collection.Actions().Delete(&oldLine).Put(&newLine).Do(ctx); err != nil {
			return err
		}
	}

	return nil
}

// addFiltersToQuery adds filters to query.
//
// Parameters:
// - query: the query to add filters to
// - filterIndex: the index of the first filter (e.g. 0 for v0, 3 for v3)
// - fieldIndex: the index of the first filter value in fieldValues
// - fieldValues: the values of the filters
//
// Returns:
// - the query with filters added.
func (a *adapter) addFiltersToQuery(query *docstore.Query, filterIndex, fieldIndex int, fieldValues ...string) *docstore.Query {
	if fieldIndex <= filterIndex && filterIndex < fieldIndex+len(fieldValues) && fieldValues[filterIndex-fieldIndex] != "" {
		query = query.Where(docstore.FieldPath(fmt.Sprintf("v%d", filterIndex)), EqualOp, fieldValues[filterIndex-fieldIndex])
	}
	return query
}

// UpdateFilteredPolicies deletes old rules and adds new rules.
func (a *adapter) UpdateFilteredPolicies(sec string, ptype string, newPolicies [][]string, fieldIndex int, fieldValues ...string) ([][]string, error) {
	query := a.collection.Query().Where(docstore.FieldPath("ptype"), EqualOp, ptype)

	// add filters to query
	for i := 0; i <= 5; i++ { // max 6 filters (v0-v5)
		query = a.addFiltersToQuery(query, i, fieldIndex, fieldValues...)
	}

	oldLines := make([][]string, 0)
	newLines := make([]CasbinRule, 0, len(newPolicies))
	for _, newPolicy := range newPolicies {
		newLines = append(newLines, savePolicyLine(ptype, newPolicy))
	}

	// Load and delete old policies.
	actionList := a.collection.Actions()
	ctx, cancel := context.WithTimeout(context.Background(), a.timeout)
	defer cancel()
	iter := query.Get(ctx)
	defer iter.Stop()
	for {
		var line CasbinRule
		err := iter.Next(ctx, &line)
		if err == io.EOF {
			break
		} else if err != nil {
			return nil, err
		} else {
			oldLines = append(oldLines, line.toStringPolicy())
			actionList.Delete(&line)
		}
	}

	// Insert new policies.
	for i := range newLines {
		actionList.Put(&newLines[i])
	}

	if err := actionList.Do(ctx); err != nil {
		return nil, err
	}

	return oldLines, nil
}

func (c *CasbinRule) toStringPolicy() []string {
	fields := [...]string{c.PType, c.V0, c.V1, c.V2, c.V3, c.V4, c.V5}
	policy := make([]string, 0, len(fields))

	for _, field := range fields {
		if field != "" {
			policy = append(policy, field)
		}
	}

	return policy
}
