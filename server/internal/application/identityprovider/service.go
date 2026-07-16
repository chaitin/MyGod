package identityprovider

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"unicode"

	"app/internal/store"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Dependencies struct {
	DB             *gorm.DB
	NewID          func() string
	GenerateSuffix func() (string, error)
}

type Service struct {
	db             *gorm.DB
	newID          func() string
	generateSuffix func() (string, error)
}

func NewService(deps Dependencies) *Service {
	newID := deps.NewID
	if newID == nil {
		newID = uuid.NewString
	}
	generateSuffix := deps.GenerateSuffix
	if generateSuffix == nil {
		generateSuffix = randomKeySuffix
	}
	return &Service{db: deps.DB, newID: newID, generateSuffix: generateSuffix}
}

func (s *Service) List(ctx context.Context) ([]Provider, error) {
	var storedProviders []store.ThirdPartyLoginProvider
	if err := s.db.WithContext(ctx).Order("sort_order ASC").Order("name ASC").Order("id ASC").Find(&storedProviders).Error; err != nil {
		return nil, internalError(err)
	}
	return providersFromStore(storedProviders)
}

func (s *Service) Get(ctx context.Context, providerID string) (Provider, error) {
	storedProvider, err := s.find(ctx, providerID)
	if err != nil {
		return Provider{}, err
	}
	return providerFromStore(storedProvider)
}

func (s *Service) GetEnabledByKey(ctx context.Context, key string) (Provider, error) {
	var storedProvider store.ThirdPartyLoginProvider
	err := s.db.WithContext(ctx).Where("key = ? AND enabled = ?", strings.TrimSpace(key), true).First(&storedProvider).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Provider{}, newError(CodeNotFound, "第三方登录方式不存在", err)
	}
	if err != nil {
		return Provider{}, internalError(err)
	}
	provider, err := providerFromStore(storedProvider)
	if err != nil {
		return Provider{}, internalError(err)
	}
	return provider, nil
}

func (s *Service) Create(ctx context.Context, cmd WriteCommand) (Provider, error) {
	storedProvider, err := normalizeWriteCommand(cmd)
	if err != nil {
		return Provider{}, err
	}
	storedProvider.ID = s.newID()
	storedProvider.Enabled = true
	key, err := s.generateUniqueKey(ctx, storedProvider.Name, storedProvider.Type)
	if err != nil {
		return Provider{}, internalError(err)
	}
	storedProvider.Key = key
	sortOrder, err := s.nextSortOrder(ctx)
	if err != nil {
		return Provider{}, internalError(err)
	}
	storedProvider.SortOrder = sortOrder
	if err := s.db.WithContext(ctx).Create(&storedProvider).Error; err != nil {
		if isUniqueConstraintError(err) {
			return Provider{}, newError(CodeConflict, "第三方登录方式标识已存在", err)
		}
		return Provider{}, internalError(err)
	}
	provider, err := providerFromStore(storedProvider)
	if err != nil {
		return Provider{}, internalError(err)
	}
	return provider, nil
}

func (s *Service) Update(ctx context.Context, cmd UpdateCommand) (Provider, error) {
	storedProvider, err := s.find(ctx, cmd.ProviderID)
	if err != nil {
		return Provider{}, err
	}
	updatedProvider, err := normalizeWriteCommand(cmd.WriteCommand)
	if err != nil {
		return Provider{}, err
	}
	if err := s.db.WithContext(ctx).Model(&storedProvider).Updates(map[string]any{
		"name": updatedProvider.Name, "type": updatedProvider.Type,
		"client_id": updatedProvider.ClientID, "client_secret": updatedProvider.ClientSecret,
		"scopes": updatedProvider.Scopes, "config": updatedProvider.Config,
	}).Error; err != nil {
		if isUniqueConstraintError(err) {
			return Provider{}, newError(CodeConflict, "第三方登录方式标识已存在", err)
		}
		return Provider{}, internalError(err)
	}
	return s.reload(ctx, storedProvider.ID)
}

func (s *Service) SetEnabled(ctx context.Context, cmd SetEnabledCommand) (Provider, error) {
	storedProvider, err := s.find(ctx, cmd.ProviderID)
	if err != nil {
		return Provider{}, err
	}
	if err := s.db.WithContext(ctx).Model(&storedProvider).Update("enabled", cmd.Enabled).Error; err != nil {
		return Provider{}, internalError(err)
	}
	storedProvider.Enabled = cmd.Enabled
	provider, err := providerFromStore(storedProvider)
	if err != nil {
		return Provider{}, internalError(err)
	}
	return provider, nil
}

func (s *Service) Move(ctx context.Context, cmd MoveCommand) ([]Provider, error) {
	id, err := normalizeID(cmd.ProviderID)
	if err != nil {
		return nil, err
	}
	direction := strings.TrimSpace(cmd.Direction)
	if direction != "up" && direction != "down" {
		return nil, newError(CodeInvalidRequest, "移动方向只能是 up 或 down", nil)
	}
	var result []Provider
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var storedProviders []store.ThirdPartyLoginProvider
		if err := tx.Order("sort_order ASC").Order("name ASC").Order("id ASC").Find(&storedProviders).Error; err != nil {
			return err
		}
		index := -1
		for currentIndex, provider := range storedProviders {
			if provider.ID == id {
				index = currentIndex
				break
			}
		}
		if index == -1 {
			return gorm.ErrRecordNotFound
		}
		targetIndex := index
		if direction == "up" && index > 0 {
			targetIndex = index - 1
		}
		if direction == "down" && index < len(storedProviders)-1 {
			targetIndex = index + 1
		}
		storedProviders[index], storedProviders[targetIndex] = storedProviders[targetIndex], storedProviders[index]
		result = make([]Provider, 0, len(storedProviders))
		for currentIndex := range storedProviders {
			sortOrder := (currentIndex + 1) * 10
			if err := tx.Model(&store.ThirdPartyLoginProvider{}).Where("id = ?", storedProviders[currentIndex].ID).
				Update("sort_order", sortOrder).Error; err != nil {
				return err
			}
			storedProviders[currentIndex].SortOrder = sortOrder
			provider, err := providerFromStore(storedProviders[currentIndex])
			if err != nil {
				return err
			}
			result = append(result, provider)
		}
		return nil
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, newError(CodeNotFound, "第三方登录方式不存在", err)
	}
	if err != nil {
		return nil, internalError(err)
	}
	return result, nil
}

func (s *Service) Delete(ctx context.Context, providerID string) error {
	storedProvider, err := s.find(ctx, providerID)
	if err != nil {
		return err
	}
	if err := s.db.WithContext(ctx).Delete(&storedProvider).Error; err != nil {
		return internalError(err)
	}
	return nil
}

func (s *Service) find(ctx context.Context, providerID string) (store.ThirdPartyLoginProvider, error) {
	id, err := normalizeID(providerID)
	if err != nil {
		return store.ThirdPartyLoginProvider{}, err
	}
	var storedProvider store.ThirdPartyLoginProvider
	err = s.db.WithContext(ctx).First(&storedProvider, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return store.ThirdPartyLoginProvider{}, newError(CodeNotFound, "第三方登录方式不存在", err)
	}
	if err != nil {
		return store.ThirdPartyLoginProvider{}, internalError(err)
	}
	return storedProvider, nil
}

func (s *Service) reload(ctx context.Context, providerID string) (Provider, error) {
	var storedProvider store.ThirdPartyLoginProvider
	if err := s.db.WithContext(ctx).First(&storedProvider, "id = ?", providerID).Error; err != nil {
		return Provider{}, internalError(err)
	}
	provider, err := providerFromStore(storedProvider)
	if err != nil {
		return Provider{}, internalError(err)
	}
	return provider, nil
}

func normalizeWriteCommand(cmd WriteCommand) (store.ThirdPartyLoginProvider, error) {
	name := strings.TrimSpace(cmd.Name)
	if name == "" {
		return store.ThirdPartyLoginProvider{}, newError(CodeInvalidRequest, "名称不能为空", nil)
	}
	providerType, err := NormalizeType(cmd.Type)
	if err != nil {
		return store.ThirdPartyLoginProvider{}, newError(CodeInvalidRequest, err.Error(), err)
	}
	clientID := strings.TrimSpace(cmd.ClientID)
	if clientID == "" {
		return store.ThirdPartyLoginProvider{}, newError(CodeInvalidRequest, "Client ID 不能为空", nil)
	}
	clientSecret := strings.TrimSpace(cmd.ClientSecret)
	if clientSecret == "" {
		return store.ThirdPartyLoginProvider{}, newError(CodeInvalidRequest, "Client Secret 不能为空", nil)
	}
	scopes, err := NormalizeScopes(providerType, cmd.Scopes)
	if err != nil {
		return store.ThirdPartyLoginProvider{}, newError(CodeInvalidRequest, err.Error(), err)
	}
	scopesJSON, err := json.Marshal(scopes)
	if err != nil {
		return store.ThirdPartyLoginProvider{}, internalError(err)
	}
	config, err := NormalizeConfig(providerType, cmd.Config)
	if err != nil {
		return store.ThirdPartyLoginProvider{}, newError(CodeInvalidRequest, err.Error(), err)
	}
	configJSON, err := json.Marshal(config)
	if err != nil {
		return store.ThirdPartyLoginProvider{}, internalError(err)
	}
	return store.ThirdPartyLoginProvider{
		Name: name, Type: providerType, ClientID: clientID, ClientSecret: clientSecret,
		Scopes: scopesJSON, Config: configJSON,
	}, nil
}

func providerFromStore(value store.ThirdPartyLoginProvider) (Provider, error) {
	var scopes []string
	if len(value.Scopes) > 0 {
		if err := json.Unmarshal(value.Scopes, &scopes); err != nil {
			return Provider{}, err
		}
	}
	if scopes == nil {
		scopes = []string{}
	}
	config := map[string]any{}
	if len(value.Config) > 0 {
		if err := json.Unmarshal(value.Config, &config); err != nil {
			return Provider{}, err
		}
	}
	if config == nil {
		config = map[string]any{}
	}
	return Provider{
		ID: value.ID, Name: value.Name, Key: value.Key, Type: value.Type, Enabled: value.Enabled,
		ClientID: value.ClientID, ClientSecret: value.ClientSecret, Scopes: scopes, Config: config,
		SortOrder: value.SortOrder,
	}, nil
}

func providersFromStore(values []store.ThirdPartyLoginProvider) ([]Provider, error) {
	result := make([]Provider, 0, len(values))
	for _, value := range values {
		provider, err := providerFromStore(value)
		if err != nil {
			return nil, internalError(err)
		}
		result = append(result, provider)
	}
	return result, nil
}

func normalizeID(value string) (string, error) {
	id := strings.TrimSpace(value)
	if _, err := uuid.Parse(id); err != nil {
		return "", newError(CodeInvalidRequest, "第三方登录方式 ID 格式错误", err)
	}
	return id, nil
}

func (s *Service) generateUniqueKey(ctx context.Context, name string, providerType string) (string, error) {
	base := slugifyKey(name)
	if base == "" {
		base = slugifyKey(providerType)
	}
	if base == "" {
		base = "third-party"
	}
	if len(base) > 80 {
		base = strings.Trim(base[:80], "-_")
	}
	if base == "" {
		base = "third-party"
	}
	key := base
	for attempt := 0; attempt < 8; attempt++ {
		var count int64
		if err := s.db.WithContext(ctx).Model(&store.ThirdPartyLoginProvider{}).Where("key = ?", key).Count(&count).Error; err != nil {
			return "", err
		}
		if count == 0 {
			return key, nil
		}
		suffix, err := s.generateSuffix()
		if err != nil {
			return "", err
		}
		prefixMaxLength := 80 - len(suffix) - 1
		prefix := base
		if len(prefix) > prefixMaxLength {
			prefix = strings.Trim(prefix[:prefixMaxLength], "-_")
		}
		if prefix == "" {
			prefix = "third-party"
		}
		key = prefix + "-" + suffix
	}
	return "", errors.New("generate unique third-party provider key")
}

func (s *Service) nextSortOrder(ctx context.Context) (int, error) {
	var maxSortOrder sql.NullInt64
	if err := s.db.WithContext(ctx).Model(&store.ThirdPartyLoginProvider{}).Select("MAX(sort_order)").Scan(&maxSortOrder).Error; err != nil {
		return 0, err
	}
	if !maxSortOrder.Valid {
		return 10, nil
	}
	return int(maxSortOrder.Int64) + 10, nil
}

func slugifyKey(name string) string {
	var builder strings.Builder
	lastSeparator := true
	for _, currentRune := range strings.ToLower(name) {
		switch {
		case currentRune >= 'a' && currentRune <= 'z', currentRune >= '0' && currentRune <= '9':
			builder.WriteRune(currentRune)
			lastSeparator = false
		case currentRune == '-' || currentRune == '_' || unicode.IsSpace(currentRune):
			if !lastSeparator {
				builder.WriteByte('-')
				lastSeparator = true
			}
		}
	}
	return strings.Trim(builder.String(), "-_")
}

func randomKeySuffix() (string, error) {
	var value [4]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(value[:]), nil
}

func isUniqueConstraintError(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "unique") || strings.Contains(message, "duplicate")
}

var _ AdminService = (*Service)(nil)
var _ LoginProviderService = (*Service)(nil)
