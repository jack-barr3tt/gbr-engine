package utils

import (
	"encoding/json"

	"github.com/jack-barr3tt/gbr-engine/src/common/types"
)

func UnmarshalTrustMessages(data string) ([]types.TrustMessage, error) {
	var messages []types.TrustMessage
	err := json.Unmarshal([]byte(data), &messages)
	return messages, err
}

func UnmarshalTDMessages(data string) ([]types.TDCMsgBody, []types.TDSMsgBody, error) {
	var raws []json.RawMessage
	if err := json.Unmarshal([]byte(data), &raws); err != nil {
		return nil, nil, err
	}

	var tdCMsgs []types.TDCMsgBody
	var tdSMsgs []types.TDSMsgBody

	for _, raw := range raws {
		var tdC types.TDCMsgEnvelope
		if err := json.Unmarshal(raw, &tdC); err == nil {
			if tdC.CAMsgBody != nil {
				tdCMsgs = append(tdCMsgs, *tdC.CAMsgBody)
			}
			if tdC.CBMsgBody != nil {
				tdCMsgs = append(tdCMsgs, *tdC.CBMsgBody)
			}
			if tdC.CCMsgBody != nil {
				tdCMsgs = append(tdCMsgs, *tdC.CCMsgBody)
			}
			if tdC.CTMsgBody != nil {
				tdCMsgs = append(tdCMsgs, *tdC.CTMsgBody)
			}
			continue
		}

		var tdS types.TDSMsgEnvelope
		if err := json.Unmarshal(raw, &tdS); err == nil {
			if tdS.SFMsgBody != nil {
				tdSMsgs = append(tdSMsgs, *tdS.SFMsgBody)
			}
			if tdS.SGMsgBody != nil {
				tdSMsgs = append(tdSMsgs, *tdS.SGMsgBody)
			}
			if tdS.SHMsgBody != nil {
				tdSMsgs = append(tdSMsgs, *tdS.SHMsgBody)
			}
			continue
		}
	}

	return tdCMsgs, tdSMsgs, nil
}

func UnmarshalVSTP(jsonStr string) (*types.VSTPMessage, error) {
	var vstpMsg types.VSTPMessage
	err := json.Unmarshal([]byte(jsonStr), &vstpMsg)
	if err != nil {
		return nil, err
	}
	return &vstpMsg, nil
}
