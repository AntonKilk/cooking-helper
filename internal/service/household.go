package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/AntonKilk/cooking-helper/internal/domain"
	"github.com/AntonKilk/cooking-helper/internal/repository"
)

// ErrInvalidFamilySize is returned when the requested family composition falls
// outside the supported range (adults 1-6, kids 0-6).
var ErrInvalidFamilySize = errors.New("service: family size out of range")

// ErrEmptyIngredient is returned when an add (pantry basic or disliked) carries
// only whitespace.
var ErrEmptyIngredient = errors.New("service: empty ingredient")

// Family size bounds enforced on every profile update. Defaults seed a brand-new
// household on first access.
const (
	minAdults     = 1
	maxAdults     = 6
	minKids       = 0
	maxKids       = 6
	defaultAdults = 2
	defaultKids   = 0
)

// householdRepo is the subset of repository.Store the service depends on, kept
// narrow so the service can be unit-tested with a fake. *repository.Store
// satisfies it.
type householdRepo interface {
	FirstHousehold(ctx context.Context) (*domain.HouseholdProfile, error)
	CreateHousehold(ctx context.Context, h *domain.HouseholdProfile) error
	GetHousehold(ctx context.Context, id string) (*domain.HouseholdProfile, error)
	UpdateHousehold(ctx context.Context, h *domain.HouseholdProfile) error
}

// HouseholdService orchestrates household-profile reads and writes for the single
// MVP household, applying defaults and validation around the repository.
type HouseholdService struct {
	repo householdRepo
}

// NewHouseholdService returns a service backed by the given repository.
func NewHouseholdService(repo householdRepo) *HouseholdService {
	return &HouseholdService{repo: repo}
}

// Current returns the single household profile, creating it with defaults
// (defaultAdults / defaultKids, defaultLang) on first access.
func (s *HouseholdService) Current(ctx context.Context, defaultLang domain.Language) (*domain.HouseholdProfile, error) {
	h, err := s.repo.FirstHousehold(ctx)
	if errors.Is(err, repository.ErrNotFound) {
		h = &domain.HouseholdProfile{
			Language:     defaultLang,
			FamilySize:   domain.FamilySize{Adults: defaultAdults, Kids: defaultKids},
			PantryBasics: domain.DefaultPantryBasics(defaultLang),
		}
		if err := s.repo.CreateHousehold(ctx, h); err != nil {
			return nil, fmt.Errorf("current household: %w", err)
		}
		return h, nil
	}
	if err != nil {
		return nil, fmt.Errorf("current household: %w", err)
	}
	return h, nil
}

// UpdateProfile validates and applies a family-composition and language change to
// the household identified by id, preserving its disliked-ingredient and pantry
// lists. It returns ErrInvalidFamilySize when the sizes are out of range.
func (s *HouseholdService) UpdateProfile(ctx context.Context, id string, lang domain.Language, adults, kids int) (*domain.HouseholdProfile, error) {
	if adults < minAdults || adults > maxAdults || kids < minKids || kids > maxKids {
		return nil, ErrInvalidFamilySize
	}

	h, err := s.repo.GetHousehold(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("update profile: %w", err)
	}

	h.Language = lang
	h.FamilySize = domain.FamilySize{Adults: adults, Kids: kids}
	if err := s.repo.UpdateHousehold(ctx, h); err != nil {
		return nil, fmt.Errorf("update profile: %w", err)
	}
	return h, nil
}

// AddDisliked appends a disliked ingredient to the household, trimming the term
// and deduplicating case-insensitively. A blank term yields ErrEmptyIngredient.
// Adding an already-present term is a no-op success (idempotent): the current
// profile is returned without a write.
func (s *HouseholdService) AddDisliked(ctx context.Context, id, term string) (*domain.HouseholdProfile, error) {
	term = strings.TrimSpace(term)
	if term == "" {
		return nil, ErrEmptyIngredient
	}

	h, err := s.repo.GetHousehold(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("add disliked: %w", err)
	}

	for _, existing := range h.DislikedIngredients {
		if strings.EqualFold(strings.TrimSpace(existing), term) {
			return h, nil
		}
	}

	h.DislikedIngredients = append(h.DislikedIngredients, term)
	if err := s.repo.UpdateHousehold(ctx, h); err != nil {
		return nil, fmt.Errorf("add disliked: %w", err)
	}
	return h, nil
}

// RemoveDisliked drops every disliked ingredient matching term case-insensitively
// and persists the result. Removing an absent term is a no-op success. An empty
// list is valid.
func (s *HouseholdService) RemoveDisliked(ctx context.Context, id, term string) (*domain.HouseholdProfile, error) {
	term = strings.TrimSpace(term)

	h, err := s.repo.GetHousehold(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("remove disliked: %w", err)
	}

	kept := make([]string, 0, len(h.DislikedIngredients))
	for _, existing := range h.DislikedIngredients {
		if strings.EqualFold(strings.TrimSpace(existing), term) {
			continue
		}
		kept = append(kept, existing)
	}
	h.DislikedIngredients = kept

	if err := s.repo.UpdateHousehold(ctx, h); err != nil {
		return nil, fmt.Errorf("remove disliked: %w", err)
	}
	return h, nil
}

// AddPantryBasic appends a staple to the household's pantry-basics list and
// persists it. The item is trimmed; an all-whitespace item yields
// ErrEmptyIngredient. A case-insensitive duplicate is ignored, leaving the list
// unchanged. The updated profile is returned.
func (s *HouseholdService) AddPantryBasic(ctx context.Context, id, item string) (*domain.HouseholdProfile, error) {
	item = strings.TrimSpace(item)
	if item == "" {
		return nil, ErrEmptyIngredient
	}

	h, err := s.repo.GetHousehold(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("add pantry basic: %w", err)
	}

	for _, existing := range h.PantryBasics {
		if strings.EqualFold(strings.TrimSpace(existing), item) {
			return h, nil
		}
	}

	h.PantryBasics = append(h.PantryBasics, item)
	if err := s.repo.UpdateHousehold(ctx, h); err != nil {
		return nil, fmt.Errorf("add pantry basic: %w", err)
	}
	return h, nil
}

// RemovePantryBasic drops the case-insensitive match of item from the household's
// pantry-basics list and persists the result. Removing an absent item is a no-op
// (still persisted), keeping the operation idempotent. The updated profile is
// returned.
func (s *HouseholdService) RemovePantryBasic(ctx context.Context, id, item string) (*domain.HouseholdProfile, error) {
	item = strings.TrimSpace(item)

	h, err := s.repo.GetHousehold(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("remove pantry basic: %w", err)
	}

	kept := make([]string, 0, len(h.PantryBasics))
	for _, existing := range h.PantryBasics {
		if strings.EqualFold(strings.TrimSpace(existing), item) {
			continue
		}
		kept = append(kept, existing)
	}
	h.PantryBasics = kept

	if err := s.repo.UpdateHousehold(ctx, h); err != nil {
		return nil, fmt.Errorf("remove pantry basic: %w", err)
	}
	return h, nil
}
