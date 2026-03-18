package types

type PassengerTrainConsistMessage struct {
	MessageHeader                    GeminiMessageHeader                    `xml:"MessageHeader"`
	MessageStatus                    string                                 `xml:"MessageStatus"`
	TrainOperationalIdentification   GeminiTrainOperationalIdentification   `xml:"TrainOperationalIdentification"`
	OperationalTrainNumberIdentifier GeminiOperationalTrainNumberIdentifier `xml:"OperationalTrainNumberIdentifier"`
	ResponsibleRU                    string                                 `xml:"ResponsibleRU,omitempty"`
	Allocations                      []GeminiAllocation                     `xml:"Allocation"`
}

type GeminiMessageHeader struct {
	MessageReference GeminiMessageReference `xml:"MessageReference"`
	MessageRoutingID string                 `xml:"MessageRoutingID,omitempty"`
	SenderReference  string                 `xml:"SenderReference,omitempty"`
	Sender           GeminiParty            `xml:"Sender"`
	Recipient        GeminiParty            `xml:"Recipient"`
}

type GeminiMessageReference struct {
	MessageType        string `xml:"MessageType"`
	MessageTypeVersion string `xml:"MessageTypeVersion"`
	MessageIdentifier  string `xml:"MessageIdentifier"`
	MessageDateTime    string `xml:"MessageDateTime"`
}

type GeminiParty struct {
	CIInstanceNumber string `xml:"CI_InstanceNumber,attr"`
	Value            string `xml:",chardata"`
}

type GeminiTrainOperationalIdentification struct {
	Transports []GeminiTransportOperationalIdentifiers `xml:"TransportOperationalIdentifiers"`
}

type GeminiTransportOperationalIdentifiers struct {
	ObjectType    string `xml:"ObjectType"`
	Company       string `xml:"Company"`
	Core          string `xml:"Core"`
	Variant       string `xml:"Variant"`
	TimetableYear string `xml:"TimetableYear"`
	StartDate     string `xml:"StartDate"`
}

type GeminiOperationalTrainNumberIdentifier struct {
	OperationalTrainNumber      string `xml:"OperationalTrainNumber"`
	ScheduledTimeAtHandover     string `xml:"ScheduledTimeAtHandover,omitempty"`
	ScheduledDateTimeAtTransfer string `xml:"ScheduledDateTimeAtTransfer,omitempty"`
}

type GeminiAllocation struct {
	AllocationSequenceNumber int64 `xml:"AllocationSequenceNumber"`

	TrainOriginDateTime   string               `xml:"TrainOriginDateTime,omitempty"`
	TrainOriginLocation   *GeminiLocationIdent `xml:"TrainOriginLocation,omitempty"`
	ResourceGroupPosition string               `xml:"ResourceGroupPosition,omitempty"`
	DiagramDate           string               `xml:"DiagramDate,omitempty"`
	DiagramNo             string               `xml:"DiagramNo,omitempty"`
	TrainDestLocation     *GeminiLocationIdent `xml:"TrainDestLocation,omitempty"`
	TrainDestDateTime     string               `xml:"TrainDestDateTime,omitempty"`

	AllocationOriginLocation      *GeminiLocationIdent `xml:"AllocationOriginLocation,omitempty"`
	AllocationOriginDateTime      string               `xml:"AllocationOriginDateTime,omitempty"`
	AllocationOriginMiles         string               `xml:"AllocationOriginMiles,omitempty"`
	AllocationDestinationLocation *GeminiLocationIdent `xml:"AllocationDestinationLocation,omitempty"`
	AllocationDestinationDateTime string               `xml:"AllocationDestinationDateTime,omitempty"`
	AllocationDestinationMiles    string               `xml:"AllocationDestinationMiles,omitempty"`

	Reversed      string              `xml:"Reversed,omitempty"`
	ResourceGroup GeminiResourceGroup `xml:"ResourceGroup"`
}

type GeminiLocationIdent struct {
	CountryCodeISO                   string                                `xml:"CountryCodeISO"`
	LocationPrimaryCode              string                                `xml:"LocationPrimaryCode"`
	PrimaryLocationName              string                                `xml:"PrimaryLocationName,omitempty"`
	LocationSubsidiaryIdentification *GeminiLocationSubsidiaryIdentification `xml:"LocationSubsidiaryIdentification,omitempty"`
}

type GeminiLocationSubsidiaryIdentification struct {
	LocationSubsidiaryCode GeminiSubsidiaryCode `xml:"LocationSubsidiaryCode"`
	AllocationCompany      string               `xml:"AllocationCompany"`
	LocationSubsidiaryName string               `xml:"LocationSubsidiaryName,omitempty"`
}

type GeminiSubsidiaryCode struct {
	LocationSubsidiaryTypeCode string `xml:"LocationSubsidiaryTypeCode,attr"`
	Text                       string `xml:",chardata"`
}

type GeminiResourceGroup struct {
	ResourceGroupId       string               `xml:"ResourceGroupId"`
	TypeOfResource        string               `xml:"TypeOfResource"`
	FleetID               string               `xml:"FleetId,omitempty"`
	ResourceGroupStatus   string               `xml:"ResourceGroupStatus,omitempty"`
	EndOfDayMiles         string               `xml:"EndOfDayMiles,omitempty"`
	Preassignment         GeminiPreassignment  `xml:"Preassignment"`
	NextAvailableLocation *GeminiLocationIdent `xml:"NextAvailableLocation,omitempty"`
	NextAvailableDateTime string               `xml:"NextAvailableDateTime,omitempty"`
	Restrictions          []GeminiRestriction  `xml:"Restriction"`
	Vehicles              []GeminiVehicle      `xml:"Vehicle"`
}

type GeminiPreassignment struct {
	PreAssignmentRequiredLocation *GeminiLocationIdent `xml:"PreAssignmentRequiredLocation,omitempty"`
	PreAssignmentDueDateTime      string               `xml:"PreAssignmentDueDateTime,omitempty"`
	PreAssignmentReason           string               `xml:"PreAssignmentReason,omitempty"`
	PreAssignmentDateTime         string               `xml:"PreAssignmentDateTime,omitempty"`
}

type GeminiRestriction struct {
	RestrictionCode     string `xml:"RestrictionCode"`
	Date                string `xml:"Date"`
	RestrictionComments string `xml:"RestrictionComments"`
	Miles               string `xml:"Miles,omitempty"`
	Hours               string `xml:"Hours,omitempty"`
}

type GeminiVehicle struct {
	VehicleId                string               `xml:"VehicleId"`
	TypeOfVehicle            string               `xml:"TypeOfVehicle"`
	ResourcePosition         string               `xml:"ResourcePosition,omitempty"`
	PlannedResourceGroup     string               `xml:"PlannedResourceGroup,omitempty"`
	Control                  *GeminiControl       `xml:"Control,omitempty"`
	SpecificType             string               `xml:"SpecificType,omitempty"`
	Length                   *GeminiMeasureLength `xml:"Length,omitempty"`
	Weight                   string               `xml:"Weight,omitempty"`
	Livery                   string               `xml:"Livery,omitempty"`
	Decor                    string               `xml:"Decor,omitempty"`
	SpecialCharacteristics   string               `xml:"SpecialCharacteristics,omitempty"`
	Smoking                  string               `xml:"Smoking,omitempty"`
	ElectricTrainHeating     string               `xml:"ElectricTrainHeating,omitempty"`
	NumberOfSeats            string               `xml:"NumberOfSeats,omitempty"`
	VehicleStatus            string               `xml:"VehicleStatus,omitempty"`
	NextAvailableLocation    *GeminiLocationIdent `xml:"NextAvailableLocation,omitempty"`
	NextAvailableDateTime    string               `xml:"NextAvailableDateTime,omitempty"`
	RegisteredStatus         string               `xml:"RegisteredStatus,omitempty"`
	RadioNumberA             string               `xml:"RadioNumberA,omitempty"`
	RadioNumberB             string               `xml:"RadioNumberB,omitempty"`
	CoachLetter              string               `xml:"CoachLetter,omitempty"`
	FuelCapacity             string               `xml:"FuelCapacity,omitempty"`
	Cabs                     string               `xml:"Cabs,omitempty"`
	DateEnteredService       string               `xml:"DateEnteredService,omitempty"`
	DateRegistered           string               `xml:"DateRegistered,omitempty"`
	DateRegistrationExpires  string               `xml:"DateRegistrationExpires,omitempty"`
	CellphoneNumber          string               `xml:"CellphoneNumber,omitempty"`
	RegisteredCategory       string               `xml:"RegisteredCategory,omitempty"`
	ExamPlanningIndicator    string               `xml:"ExamPlanningIndicator,omitempty"`
	ModificationIndicators   string               `xml:"ModificationIndicators,omitempty"`
	VehicleName              string               `xml:"VehicleName,omitempty"`
	TrainBrakeType           string               `xml:"TrainBrakeType,omitempty"`
	MaximumSpeed             string               `xml:"MaximumSpeed,omitempty"`
	OperatorCode             string               `xml:"OperatorCode,omitempty"`
	OwnerCode                string               `xml:"OwnerCode,omitempty"`
	HirerCode                string               `xml:"HirerCode,omitempty"`
	SubHirerCode             string               `xml:"SubHirerCode,omitempty"`
	Defects                  []GeminiDefect       `xml:"Defect"`
}

type GeminiControl struct {
	Code        string `xml:"Code,attr"`
	Description string `xml:"Description,attr"`
}

type GeminiMeasureLength struct {
	Value   string `xml:"Value"`
	Measure string `xml:"Measure"`
}

type GeminiDefect struct {
	MaintenanceUID            string `xml:"MaintenanceUID"`
	DefectCode                string `xml:"DefectCode"`
	MaintenanceDefectLocation string `xml:"MaintenanceDefectLocation,omitempty"`
	DefectDescription         string `xml:"DefectDescription"`
	DefectStatus              string `xml:"DefectStatus"`
	MaintenanceRepairLocation string `xml:"MaintenanceRepairLocation,omitempty"`
	RepairDescription         string `xml:"RepairDescription,omitempty"`
}
