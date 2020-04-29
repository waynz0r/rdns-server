package keyvalue

import (
	"time"

	"github.com/rancher/rdns-server/model"
)

const (
	DriverName = "keyvalue"

	TokenValueType        = "token"
	FrozenPrefixValueType = "frozen-prefix"
	SubARecordValueType   = "sub-a-record"
	ARecordValueType      = "a-record"
	CNAMERecordValueType  = "cname-record"
	TXTRecordValueType    = "txt-record"
)

var ValueTypes = []string{FrozenPrefixValueType, SubARecordValueType, ARecordValueType, CNAMERecordValueType, TXTRecordValueType, TokenValueType}

type KeyValueStore interface {
	GetValue(name, valueType string, metadata interface{}) (string, error)
	SetValue(name, valueType string, metadata interface{}) error
	UpdateValue(name, valueType string, metadata interface{}) error
	DeleteValue(name, valueType string) error
	ListValues(valueType string) ([]string, error)
	GetExpiredValues(valueType string, t *time.Time) ([]string, error)
}

type KeyValueBackend struct {
	store KeyValueStore
}

type BaseModel struct {
	CreatedOn int64 `json:"createdOn,omitempty"`
}

type FrozenPrefix struct {
	Token     string `json:"token,omitempty"`
	CreatedOn int64  `json:"createdOn,omitempty"`
}

type Token struct {
	Token     string `json:"token,omitempty"`
	Fqdn      string `json:"fqdn,omitempty"`
	CreatedOn int64  `json:"createdOn,omitempty"`
}

func NewKeyValueBackend(store KeyValueStore) *KeyValueBackend {
	return &KeyValueBackend{
		store: store,
	}
}

func (d *KeyValueBackend) InsertFrozen(prefix string) error {
	return d.store.SetValue(prefix, FrozenPrefixValueType, FrozenPrefix{
		CreatedOn: time.Now().UnixNano(),
	})
}

func (d *KeyValueBackend) QueryFrozen(prefix string) (string, error) {
	var metadata *FrozenPrefix
	_, err := d.store.GetValue(prefix, FrozenPrefixValueType, metadata)
	if err != nil {
		return "", err
	}

	if metadata == nil {
		return "", nil
	}

	return prefix, nil
}

func (d *KeyValueBackend) RenewFrozen(prefix string) error {
	var metadata *FrozenPrefix
	_, err := d.store.GetValue(prefix, FrozenPrefixValueType, metadata)
	if err != nil {
		return err
	}

	if metadata == nil {
		return nil
	}

	metadata.CreatedOn = time.Now().UnixNano()

	err = d.store.SetValue(prefix, FrozenPrefixValueType, metadata)
	if err != nil {
		return err
	}

	return nil

	// return errors.New("invalid frozen prefix")
}

func (d *KeyValueBackend) DeleteFrozen(prefix string) error {
	return d.store.DeleteValue(prefix, FrozenPrefixValueType)
}

func (d *KeyValueBackend) DeleteExpiredFrozen(t *time.Time) error {
	expired, err := d.store.GetExpiredValues(FrozenPrefixValueType, t)
	if err != nil {
		return err
	}

	for _, name := range expired {
		err = d.store.DeleteValue(name, FrozenPrefixValueType)
		if err != nil {
			return err
		}
	}

	return nil
}

func (d *KeyValueBackend) MigrateFrozen(prefix string, expiration int64) error {
	return d.store.SetValue(prefix, FrozenPrefixValueType, FrozenPrefix{
		CreatedOn: expiration,
	})
}

func (d *KeyValueBackend) InsertToken(token, name string) (int64, error) {
	t := &Token{
		Token:     token,
		Fqdn:      name,
		CreatedOn: time.Now().UnixNano(),
	}
	return t.CreatedOn, d.store.SetValue(name, TokenValueType, t)
}

func (d *KeyValueBackend) QueryTokenCount() (int64, error) {
	tokens, err := d.store.ListValues(TokenValueType)
	if err != nil {
		return 0, err
	}

	return int64(len(tokens)), nil
}

func (d *KeyValueBackend) QueryToken(name string) (*model.Token, error) {
	var token Token
	_, err := d.store.GetValue(name, TokenValueType, &token)
	if err != nil {
		return nil, err
	}

	return &model.Token{
		ID:        token.CreatedOn,
		Token:     token.Token,
		Fqdn:      token.Fqdn,
		CreatedOn: token.CreatedOn,
	}, nil
}

func (d *KeyValueBackend) QueryExpiredTokens(t *time.Time) ([]*model.Token, error) {
	result := make([]*model.Token, 0)

	expired, err := d.store.GetExpiredValues(TokenValueType, t)
	if err != nil {
		return nil, err
	}

	for _, name := range expired {
		token, err := d.QueryToken(name)
		if err != nil {
			return nil, err
		}
		result = append(result, token)
	}

	return result, nil
}

func (d *KeyValueBackend) RenewToken(name string) (int64, int64, error) {
	to, err := d.QueryToken(name)
	if err != nil {
		return 0, 0, err
	}

	t := &Token{
		Token:     to.Token,
		Fqdn:      to.Fqdn,
		CreatedOn: time.Now().UnixNano(),
	}

	err = d.store.UpdateValue(name, TokenValueType, t)
	if err != nil {
		return 0, 0, err
	}

	return t.CreatedOn, t.CreatedOn, nil
}

func (d *KeyValueBackend) DeleteToken(token string) error {
	names, err := d.store.ListValues(TokenValueType)
	if err != nil {
		return err
	}

	for _, name := range names {
		t, err := d.QueryToken(name)
		if err != nil {
			return err
		}
		if t.Token == token {
			err = d.store.DeleteValue(t.Fqdn, TokenValueType)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (d *KeyValueBackend) MigrateToken(token, name string, expiration int64) error {
	return d.store.SetValue(name, TokenValueType, Token{
		CreatedOn: expiration,
		Fqdn:      name,
		Token:     token,
	})
}

func (d *KeyValueBackend) InsertA(a *model.RecordA) (int64, error) {
	return 0, d.store.SetValue(a.Fqdn, ARecordValueType, a)
}

func (d *KeyValueBackend) QueryA(name string) (*model.RecordA, error) {
	var record model.RecordA
	_, err := d.store.GetValue(name, ARecordValueType, &record)
	if err != nil {
		return nil, err
	}

	return &record, nil
}

func (d *KeyValueBackend) UpdateA(a *model.RecordA) (int64, error) {
	return 0, d.store.UpdateValue(a.Fqdn, ARecordValueType, a)
}

func (d *KeyValueBackend) DeleteA(name string) error {
	return d.store.DeleteValue(name, ARecordValueType)
}

func (d *KeyValueBackend) InsertSubA(a *model.SubRecordA) (int64, error) {
	return 0, d.store.SetValue(a.Fqdn, SubARecordValueType, a)
}

func (d *KeyValueBackend) UpdateSubA(a *model.SubRecordA) (int64, error) {
	return 0, d.store.UpdateValue(a.Fqdn, SubARecordValueType, a)
}

func (d *KeyValueBackend) QuerySubA(name string) (*model.SubRecordA, error) {
	var record model.SubRecordA
	_, err := d.store.GetValue(name, SubARecordValueType, &record)
	if err != nil {
		return nil, err
	}

	return &record, nil
}

func (d *KeyValueBackend) ListSubA(id int64) ([]*model.SubRecordA, error) {
	rs := make([]*model.SubRecordA, 0)

	names, err := d.store.ListValues(SubARecordValueType)
	if err != nil {
		return nil, err
	}

	for _, name := range names {
		r, err := d.QuerySubA(name)
		if err != nil {
			return nil, err
		}
		rs = append(rs, r)
	}

	return rs, nil
}

func (d *KeyValueBackend) DeleteSubA(name string) error {
	return d.store.DeleteValue(name, SubARecordValueType)
}

func (d *KeyValueBackend) InsertCNAME(c *model.RecordCNAME) (int64, error) {
	return 0, d.store.SetValue(c.Fqdn, CNAMERecordValueType, c)
}

func (d *KeyValueBackend) UpdateCNAME(c *model.RecordCNAME) (int64, error) {
	return 0, d.store.UpdateValue(c.Fqdn, CNAMERecordValueType, c)
}

func (d *KeyValueBackend) QueryCNAME(name string) (*model.RecordCNAME, error) {
	var record model.RecordCNAME
	_, err := d.store.GetValue(name, CNAMERecordValueType, &record)
	if err != nil {
		return nil, err
	}

	return &record, nil
}

func (d *KeyValueBackend) DeleteCNAME(name string) error {
	return d.store.DeleteValue(name, CNAMERecordValueType)
}

func (d *KeyValueBackend) InsertTXT(a *model.RecordTXT) (int64, error) {
	return 0, d.store.SetValue(a.Fqdn, TXTRecordValueType, a)
}

func (d *KeyValueBackend) UpdateTXT(a *model.RecordTXT) (int64, error) {
	return 0, d.store.UpdateValue(a.Fqdn, TXTRecordValueType, a)
}

func (d *KeyValueBackend) DeleteTXT(name string) error {
	return d.store.DeleteValue(name, TXTRecordValueType)
}

func (d *KeyValueBackend) QueryTXT(name string) (*model.RecordTXT, error) {
	var record model.RecordTXT
	_, err := d.store.GetValue(name, TXTRecordValueType, &record)
	if err != nil {
		return nil, err
	}

	return &record, nil
}

func (d *KeyValueBackend) QueryExpiredTXTs(id int64) ([]*model.RecordTXT, error) {
	rs := make([]*model.RecordTXT, 0)

	names, err := d.store.ListValues(TXTRecordValueType)
	if err != nil {
		return nil, err
	}

	for _, name := range names {
		r, err := d.QueryTXT(name)
		if err != nil {
			return nil, err
		}
		if r.TID == id {
			rs = append(rs, r)
		}
	}

	return rs, nil
}

func (d *KeyValueBackend) Close() error {
	return nil
}
