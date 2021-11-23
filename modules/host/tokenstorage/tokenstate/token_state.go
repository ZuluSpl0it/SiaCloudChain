package tokenstate

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	"gitlab.com/scpcorp/ScPrime/crypto"
	"gitlab.com/scpcorp/ScPrime/modules"
	"gitlab.com/scpcorp/ScPrime/types"
)

type tokenStorageInfo struct {
	Storage        int64     `json:"storage"` // sectors * second
	LastChangeTime time.Time `json:"last_change_time"`
	SectorsNum     uint64    `json:"sectors_num"`
}

// TokenRecord include information about token record.
type TokenRecord struct {
	DownloadBytes  int64            `json:"download_bytes"`
	UploadBytes    int64            `json:"upload_bytes"`
	SectorAccesses int64            `json:"sector_accesses"`
	TokenInfo      tokenStorageInfo `json:"token_info"`
}

// AttachSectorsData include information about token sector and storing it.
type AttachSectorsData struct {
	TokenID  types.TokenID
	SectorID []byte
	// If true. keep the sector in the temporary store.
	// If false, the sector is moved from temporary store to the contract.
	// KeepInTmp bool
}

// EventTopUp change of state when token replenishment.
type EventTopUp struct {
	TokenID        types.TokenID   `json:"token_id"`
	ResourceType   types.Specifier `json:"resource_type"`
	ResourceAmount int64           `json:"resource_amount"`
}

// EventTokenDownload change of state when downloading.
type EventTokenDownload struct {
	TokenID        types.TokenID `json:"token_id"`
	DownloadBytes  int64         `json:"download_bytes"`
	SectorAccesses int64         `json:"sector_accesses"`
}

// EventAddSectors represent adding sectors to token.
type EventAddSectors struct {
	TokenID    types.TokenID `json:"token_id"`
	SectorsIDs [][]byte      `json:"sectors_ids"`
}

// EventRemoveSpecificSectors represent removing specified sectors from token.
type EventRemoveSpecificSectors struct {
	TokenID    types.TokenID `json:"token_id"`
	SectorsIDs [][]byte      `json:"sectors_ids"`
}

// EventRemoveAllSectors represent force removing sectors from token.
type EventRemoveAllSectors struct {
	TokenID    types.TokenID `json:"token_id"`
	SectorsIDs [][]byte      `json:"sectors_ids"`
}

// EventAttachSectors represent attaching sector to contract.
type EventAttachSectors struct {
	TokensSectors []AttachSectorsData `json:"tokens_sectors"`
}

// Event include state events.
type Event struct {
	EventTopUp                 *EventTopUp                 `json:"event_top_up"`
	EventTokenDownload         *EventTokenDownload         `json:"event_token_download"`
	EventAddSectors            *EventAddSectors            `json:"event_add_sectors"`
	EventRemoveSpecificSectors *EventRemoveSpecificSectors `json:"event_remove_specific_sectors"`
	EventRemoveAllSectors      *EventRemoveAllSectors      `json:"event_remove_sectors"`
	EventAttachSectors         *EventAttachSectors         `json:"event_attach_sectors"`
	Time                       time.Time                   `json:"time"`
}

type sectorsDBer interface {
	Get(tokenID types.TokenID) ([]crypto.Hash, error)
	GetLimited(tokenID types.TokenID, pageID string, limit int) ([]crypto.Hash, string, error)
	Put(tokenID types.TokenID, sectorID crypto.Hash) error
	NonexistentSectors(tokenID types.TokenID, sectorIDs []crypto.Hash) ([]crypto.Hash, error)
	HasSectors(tokenID types.TokenID, sectorIDs []crypto.Hash) ([]crypto.Hash, bool, error)
	BatchDeleteSpecific(tokenID types.TokenID, sectorIDs []crypto.Hash) error
	BatchDeleteAll(tokenID types.TokenID) error
	Close() error
}

// State representation of token storage state.
type State struct {
	Tokens map[types.TokenID]TokenRecord `json:"tokens"`
	db     sectorsDBer
}

// NewState create new state.
func NewState(dir string) (*State, error) {
	db, err := newSectorsDB(dir)
	if err != nil {
		return nil, err
	}
	return &State{
		Tokens: make(map[types.TokenID]TokenRecord),
		db:     db,
	}, nil
}

// Apply handle state events.
func (s *State) Apply(e *Event) {
	applied := 0

	if e.EventTopUp != nil {
		s.eventTopUp(e.EventTopUp)
		applied++
	}
	if e.EventTokenDownload != nil {
		s.eventTokenDownload(e.EventTokenDownload)
		applied++
	}
	if e.EventAddSectors != nil {
		s.eventAddSectors(e.EventAddSectors, e.Time)
		applied++
	}
	if e.EventRemoveSpecificSectors != nil {
		s.eventRemoveSpecificSectors(e.EventRemoveSpecificSectors, e.Time)
		applied++
	}
	if e.EventRemoveAllSectors != nil {
		s.eventRemoveAllSectors(e.EventRemoveAllSectors, e.Time)
		applied++
	}
	if e.EventAttachSectors != nil {
		s.eventAttachSectors(e.EventAttachSectors, e.Time)
		applied++
	}
	if applied != 1 {
		panic(fmt.Sprintf("want 1 subevent, got %d", applied))
	}
}

func (s *State) eventTopUp(e *EventTopUp) {
	token := s.Tokens[e.TokenID]

	switch e.ResourceType {
	case modules.DownloadBytes:
		token.DownloadBytes += e.ResourceAmount
	case modules.UploadBytes:
		token.UploadBytes += e.ResourceAmount
	case modules.SectorAccesses:
		token.SectorAccesses += e.ResourceAmount
	case modules.Storage:
		token.TokenInfo.Storage += e.ResourceAmount
	}
	s.Tokens[e.TokenID] = token
}

func (s *State) eventTokenDownload(e *EventTokenDownload) {
	token := s.Tokens[e.TokenID]
	token.DownloadBytes -= e.DownloadBytes
	token.SectorAccesses -= e.SectorAccesses
	s.Tokens[e.TokenID] = token
}

func (s *State) eventAddSectors(e *EventAddSectors, t time.Time) {
	token := s.Tokens[e.TokenID]
	token.TokenInfo.updateStorageResource(int64(len(e.SectorsIDs)), t)
	token.UploadBytes -= int64(len(e.SectorsIDs) * int(modules.SectorSize))
	s.Tokens[e.TokenID] = token

	for _, sec := range e.SectorsIDs {
		err := s.db.Put(e.TokenID, crypto.ConvertBytesToHash(sec))
		if err != nil {
			panic(err)
		}
	}
}

func (s *State) eventRemoveSpecificSectors(e *EventRemoveSpecificSectors, t time.Time) {
	token := s.Tokens[e.TokenID]
	token.TokenInfo.updateStorageResource(-int64(len(e.SectorsIDs)), t)
	s.Tokens[e.TokenID] = token
	err := s.db.BatchDeleteSpecific(e.TokenID, crypto.ConvertBytesToHashes(e.SectorsIDs))
	if err != nil {
		panic(err)
	}
}

func (s *State) eventRemoveAllSectors(e *EventRemoveAllSectors, t time.Time) {
	token := s.Tokens[e.TokenID]
	token.TokenInfo.updateStorageResource(-int64(len(e.SectorsIDs)), t)
	if token.TokenInfo.Storage < 0 {
		// EventRemoveAllSectors also resets Storage resource if it goes below zero.
		// This is needed, because in practice expiration checker (CheckExpiration of token storage)
		// can not immediately remove sectors of token running out of Storage resource.
		// In most cases, it removes them much later (e.g. in 1 hour) and produces EventRemoveAllSectors.
		// It's not a fault of the token owner in this case, it's a property of the token storage implementation.
		token.TokenInfo.Storage = 0
	}
	s.Tokens[e.TokenID] = token
	err := s.db.BatchDeleteAll(e.TokenID)
	if err != nil {
		panic(err)
	}
}

func (s *State) eventAttachSectors(e *EventAttachSectors, t time.Time) {
	sectorsToRemove := map[types.TokenID][]crypto.Hash{}
	for _, sectorData := range e.TokensSectors {
		sectorsToRemove[sectorData.TokenID] = append(sectorsToRemove[sectorData.TokenID], crypto.ConvertBytesToHash(sectorData.SectorID))
	}

	for tokenID, sectorIDs := range sectorsToRemove {
		if err := s.db.BatchDeleteSpecific(tokenID, sectorIDs); err != nil {
			panic(err)
		}

		token := s.Tokens[tokenID]
		token.TokenInfo.updateStorageResource(-1*int64(len(sectorIDs)), t)
		s.Tokens[tokenID] = token
	}
}

// LoadHistory load history in state.
func (s *State) LoadHistory(r io.Reader) error {
	decoder := json.NewDecoder(r)
	decoder.DisallowUnknownFields()

	for {
		event := &Event{}
		if err := decoder.Decode(event); err == io.EOF {
			break
		} else if err != nil {
			return err
		}
		s.Apply(event)
	}
	return nil
}

// GetSectors return sectors IDs from database by token ID.
func (s *State) GetSectors(tokenID types.TokenID) ([]crypto.Hash, error) {
	return s.db.Get(tokenID)
}

// GetLimitedSectors return paginated sectors.
func (s *State) GetLimitedSectors(tokenID types.TokenID, pageID string, limit int) ([]crypto.Hash, string, error) {
	return s.db.GetLimited(tokenID, pageID, limit)
}

// NonexistentSectors returns a list of only nonexistent sectors from sectorIDs.
// It also removes duplicates from the list.
func (s *State) NonexistentSectors(tokenID types.TokenID, sectorIDs []crypto.Hash) ([]crypto.Hash, error) {
	return s.db.NonexistentSectors(tokenID, sectorIDs)
}

// HasSectors checks if all sectors exist and returns a list of existing ones.
// It also removes duplicates from the list.
func (s *State) HasSectors(tokenID types.TokenID, sectorIDs []crypto.Hash) ([]crypto.Hash, bool, error) {
	return s.db.HasSectors(tokenID, sectorIDs)
}

func (t *tokenStorageInfo) updateStorageResource(sectorsNum int64, now time.Time) {
	stResource := int64(now.Sub(t.LastChangeTime).Seconds()) * int64(t.SectorsNum)
	t.Storage = t.Storage - stResource
	// sectorsNum can be negative for removing sectors.
	if sectorsNum < 0 {
		if uint64(-sectorsNum) > t.SectorsNum {
			t.SectorsNum = 0
		} else {
			t.SectorsNum -= uint64(-sectorsNum)
		}
	} else {
		t.SectorsNum += uint64(sectorsNum)
	}
	t.LastChangeTime = now
}

// EnoughStorageResource checks if there is enough storage resource to store existing sectors and new ones
// for one second. if the resource is less, it will return false.
func (s *State) EnoughStorageResource(id types.TokenID, sectorsNum int64, now time.Time) (enoughResource bool) {
	token := s.Tokens[id]
	currentSpentStorage := int64(now.Sub(token.TokenInfo.LastChangeTime).Seconds()) * int64(token.TokenInfo.SectorsNum)
	nextSecondStorage := 1 * (sectorsNum + int64(token.TokenInfo.SectorsNum))
	if token.TokenInfo.Storage > currentSpentStorage+nextSecondStorage {
		return true
	}
	return false
}

// Close DB connection.
func (s *State) Close() error {
	return s.db.Close()
}
