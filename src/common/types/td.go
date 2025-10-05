package types

type TDCMsgType string

const (
	MsgTypeCA TDCMsgType = "CA"
	MsgTypeCB TDCMsgType = "CB"
	MsgTypeCC TDCMsgType = "CC"
	MsgTypeCT TDCMsgType = "CT"
)

type TDCMsgBody struct {
	Time       string     `json:"time"`
	AreaID     string     `json:"area_id"`
	MsgType    TDCMsgType `json:"msg_type"`
	From       string     `json:"from,omitempty"`
	To         string     `json:"to,omitempty"`
	Descr      string     `json:"descr,omitempty"`
	ReportTime string     `json:"report_time,omitempty"`
}

type TDCMsgEnvelope struct {
	CAMsgBody *TDCMsgBody `json:"CA_MSG,omitempty"`
	CBMsgBody *TDCMsgBody `json:"CB_MSG,omitempty"`
	CCMsgBody *TDCMsgBody `json:"CC_MSG,omitempty"`
	CTMsgBody *TDCMsgBody `json:"CT_MSG,omitempty"`
}

type TDSMsgType string

const (
	MsgTypeSF TDSMsgType = "SF"
	MsgTypeSG TDSMsgType = "SG"
	MsgTypeSH TDSMsgType = "SH"
)

type TDSMsgBody struct {
	Time    string     `json:"time"`
	AreaID  string     `json:"area_id"`
	Address string     `json:"address"`
	MsgType TDSMsgType `json:"msg_type"`
	Data    string     `json:"data"`
}

type TDSMsgEnvelope struct {
	SFMsgBody *TDSMsgBody `json:"SF_MSG,omitempty"`
	SGMsgBody *TDSMsgBody `json:"SG_MSG,omitempty"`
	SHMsgBody *TDSMsgBody `json:"SH_MSG,omitempty"`
}
