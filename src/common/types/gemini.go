package types

type PassengerTrainConsistMessage struct {
	MessageHeader                  GeminiMessageHeader              `xml:"MessageHeader"`
	TrainOperationalIdentification TrainOperationalIdentification   `xml:"TrainOperationalIdentification"`
	OperationalTrainNumberIdentifier OperationalTrainNumberIdentifier `xml:"OperationalTrainNumberIdentifier"`
	Allocations                    []GeminiAllocation               `xml:"Allocation"`
}

type GeminiMessageHeader struct {
	Reference struct {
		MessageIdentifier string `xml:"MessageIdentifier"`
		MessageDateTime   string `xml:"MessageDateTime"`
		MessageType       string `xml:"MessageType"`
	} `xml:"MessageReference"`
}

type TrainOperationalIdentification struct {
	Transports []TransportOperationalIdentifiers `xml:"TransportOperationalIdentifiers"`
}

type TransportOperationalIdentifiers struct {
	Core          string `xml:"Core"`
	Variant       string `xml:"Variant"`
	TimetableYear string `xml:"TimetableYear"`
	StartDate     string `xml:"StartDate"`
	Company       string `xml:"Company"`
	ObjectType    string `xml:"ObjectType"`
}

type OperationalTrainNumberIdentifier struct {
	OperationalTrainNumber string `xml:"OperationalTrainNumber"`
}

type GeminiAllocation struct {
	Sequence         int64                `xml:"AllocationSequenceNumber"`
	ResourceGroupPos string               `xml:"ResourceGroupPosition"`
	TrainOrigin      *GeminiLocationIdent `xml:"TrainOriginLocation"`
	TrainDest        *GeminiLocationIdent `xml:"TrainDestLocation"`
	Origin           *GeminiLocationIdent `xml:"AllocationOriginLocation"`
	Dest             *GeminiLocationIdent `xml:"AllocationDestinationLocation"`
	ResourceGroup    GeminiResourceGroup  `xml:"ResourceGroup"`
}

type GeminiLocationIdent struct {
	Subsidiary GeminiLocationSubsidiary `xml:"LocationSubsidiaryIdentification"`
}

type GeminiLocationSubsidiary struct {
	Codes []GeminiSubsidiaryCode `xml:"LocationSubsidiaryCode"`
}

type GeminiSubsidiaryCode struct {
	Code string `xml:",chardata"`
}

type GeminiResourceGroup struct {
	ID             string         `xml:"ResourceGroupId"`
	TypeOfResource string         `xml:"TypeOfResource"`
	FleetID        string         `xml:"FleetId"`
	Vehicles       []GeminiVehicle `xml:"Vehicle"`
}

type GeminiVehicle struct {
	ID string `xml:"VehicleId"`
}
