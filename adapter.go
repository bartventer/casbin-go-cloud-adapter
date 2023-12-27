package adapter

import (
	"context"
	"crypto/md5"
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

// Interfaces to be implemented by the adapter
var _ persist.BatchAdapter = (*adapter)(nil)
var _ persist.FilteredAdapter = (*adapter)(nil)
var _ persist.UpdatableAdapter = (*adapter)(nil)

// adapter represents the MongoDB adapter for policy storage.
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
func NewFilteredAdapter(ctx context.Context, url string) (persist.FilteredAdapter, error) {
	a, err := NewWithOption(ctx, &Config{URL: url})
	if err != nil {
		return nil, err
	}
	a.(*adapter).filtered = true

	return a.(*adapter), nil
}

// Config is the configuration for Adapter.
type Config struct {
	Timeout    time.Duration // the timeout for any operations on the adapter
	IsFiltered bool          // whether the adapter is filtered
	URL        string        // the driver url (e.g. mongodb://localhost:27017)
}

// New is the constructor for Adapter.
func New(ctx context.Context, url string) (persist.BatchAdapter, error) {
	return NewWithOption(ctx, &Config{URL: url})
}

// NewWithOption is the constructor for Adapter with option.
func NewWithOption(ctx context.Context, config *Config) (persist.BatchAdapter, error) {
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
	var p = []string{line.PType,
		line.V0, line.V1, line.V2, line.V3, line.V4, line.V5}
	var lineText string
	if line.V5 != "" {
		lineText = strings.Join(p, ", ")
	} else if line.V4 != "" {
		lineText = strings.Join(p[:6], ", ")
	} else if line.V3 != "" {
		lineText = strings.Join(p[:5], ", ")
	} else if line.V2 != "" {
		lineText = strings.Join(p[:4], ", ")
	} else if line.V1 != "" {
		lineText = strings.Join(p[:3], ", ")
	} else if line.V0 != "" {
		lineText = strings.Join(p[:2], ", ")
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

// generateID generates an ID for a CasbinRule; use md5(line) to prevent
// overwrites of an existing item.
func generateID(line CasbinRule) string {
	data := []byte(fmt.Sprint(line))
	has := md5.Sum(data)
	return fmt.Sprintf("%x", has)
}

func savePolicyLine(ptype string, rule []string) CasbinRule {
	line := CasbinRule{
		PType: ptype,
	}

	if len(rule) > 0 {
		line.V0 = rule[0]
	}
	if len(rule) > 1 {
		line.V1 = rule[1]
	}
	if len(rule) > 2 {
		line.V2 = rule[2]
	}
	if len(rule) > 3 {
		line.V3 = rule[3]
	}
	if len(rule) > 4 {
		line.V4 = rule[4]
	}
	if len(rule) > 5 {
		line.V5 = rule[5]
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

	var lines []interface{}

	for ptype, ast := range model["p"] {
		for _, rule := range ast.Policy {
			line := savePolicyLine(ptype, rule)
			lines = append(lines, &line)
		}
	}

	for ptype, ast := range model["g"] {
		for _, rule := range ast.Policy {
			line := savePolicyLine(ptype, rule)
			lines = append(lines, &line)
		}
	}
	ctx, cancel := context.WithTimeout(context.TODO(), a.timeout)
	defer cancel()

	actionList := a.collection.Actions()
	for _, line := range lines {
		actionList.Put(line)
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
// - the query with filters added
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
	for _, newLine := range newLines {
		actionList.Put(&newLine)
	}

	if err := actionList.Do(ctx); err != nil {
		return nil, err
	}

	return oldLines, nil

}

func (c *CasbinRule) toStringPolicy() []string {
	policy := make([]string, 0)
	if c.PType != "" {
		policy = append(policy, c.PType)
	}
	if c.V0 != "" {
		policy = append(policy, c.V0)
	}
	if c.V1 != "" {
		policy = append(policy, c.V1)
	}
	if c.V2 != "" {
		policy = append(policy, c.V2)
	}
	if c.V3 != "" {
		policy = append(policy, c.V3)
	}
	if c.V4 != "" {
		policy = append(policy, c.V4)
	}
	if c.V5 != "" {
		policy = append(policy, c.V5)
	}
	return policy
}
