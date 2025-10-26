package api

// Type aliases to expose common types in the api package
import api_types "github.com/jack-barr3tt/gbr-engine/src/common/api-types"

type (
	ErrorResponse               = api_types.ErrorResponse
	HealthResponse              = api_types.HealthResponse
	Location                    = api_types.Location
	LocationServicesResponse    = api_types.LocationServicesResponse
	NotFoundResponse            = api_types.NotFoundResponse
	Operator                    = api_types.Operator
	ScheduleLocation            = api_types.ScheduleLocation
	ServiceResponse             = api_types.ServiceResponse
	GetServiceParams            = api_types.GetServiceParams
	GetServicesAtLocationParams = api_types.GetServicesAtLocationParams
)
