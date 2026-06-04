package domain

type CreateAddressRequest struct {
	Label          string  `json:"label"           binding:"required,max=40"`
	RecipientName  string  `json:"recipient_name"  binding:"required,min=2,max=120"`
	RecipientPhone string  `json:"recipient_phone" binding:"required,e164"`
	AddressLine    string  `json:"address_line"    binding:"required,max=255"`
	Ward           string  `json:"ward"            binding:"required,max=80"`
	District       string  `json:"district"        binding:"required,max=80"`
	City           string  `json:"city"            binding:"required,max=80"`
	CityCode       *string `json:"city_code"       binding:"required"`
	DistrictCode   *string `json:"district_code"   binding:"required"`
	WardCode       *string `json:"ward_code"       binding:"required"`
	Country        string  `json:"country"         binding:"omitempty,iso3166_1_alpha2"`
	PostalCode     *string `json:"postal_code"     binding:"omitempty,max=20"`
	Note           *string `json:"note"            binding:"omitempty,max=255"`
	IsDefault      bool    `json:"is_default"`
}

type UpdateAddressRequest struct {
	Label          *string `json:"label"           binding:"omitempty,max=40"`
	RecipientName  *string `json:"recipient_name"  binding:"omitempty,min=2,max=120"`
	RecipientPhone *string `json:"recipient_phone" binding:"omitempty,e164"`
	AddressLine    *string `json:"address_line"    binding:"omitempty,max=255"`
	Ward           *string `json:"ward"            binding:"omitempty,max=80"`
	District       *string `json:"district"        binding:"omitempty,max=80"`
	City           *string `json:"city"            binding:"omitempty,max=80"`
	CityCode       *string `json:"city_code"       binding:"required"`
	DistrictCode   *string `json:"district_code"   binding:"required"`
	WardCode       *string `json:"ward_code"       binding:"required"`
	Country        *string `json:"country"         binding:"omitempty,iso3166_1_alpha2"`
	PostalCode     *string `json:"postal_code"     binding:"omitempty,max=20"`
	Note           *string `json:"note"            binding:"omitempty,max=255"`
	IsDefault      *bool   `json:"is_default"`
}

type AddressResponse struct {
	ID             string  `json:"id"`
	Label          string  `json:"label"`
	RecipientName  string  `json:"recipient_name"`
	RecipientPhone string  `json:"recipient_phone"`
	AddressLine    string  `json:"address_line"`
	Ward           string  `json:"ward"`
	District       string  `json:"district"`
	City           string  `json:"city"`
	Country        string  `json:"country"`
	PostalCode     *string `json:"postal_code,omitempty"`
	Note           *string `json:"note,omitempty"`
	IsDefault      bool    `json:"is_default"`
	CreatedAt      string  `json:"created_at"`
	UpdatedAt      string  `json:"updated_at"`
}

func ToAddressResponse(a *CustomerAddress) AddressResponse {
	return AddressResponse{
		ID:             a.ID.String(),
		Label:          a.Label,
		RecipientName:  a.RecipientName,
		RecipientPhone: a.RecipientPhone,
		AddressLine:    a.AddressLine,
		Ward:           a.Ward,
		District:       a.District,
		City:           a.City,
		Country:        a.Country,
		PostalCode:     a.PostalCode,
		Note:           a.Note,
		IsDefault:      a.IsDefault,
		CreatedAt:      a.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		UpdatedAt:      a.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z"),
	}
}
