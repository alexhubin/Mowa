package media

import (
	"time"

	"github.com/livekit/protocol/auth"
)

type TokenIssuer struct {
	APIKey    string
	APISecret string
	TTL       time.Duration
}

func (i TokenIssuer) Issue(room, identity, name string) (string, error) {
	grant := &auth.VideoGrant{RoomJoin: true, Room: room}
	grant.SetCanPublish(true)
	grant.SetCanSubscribe(true)
	grant.SetCanPublishData(false)

	return auth.NewAccessToken(i.APIKey, i.APISecret).
		SetVideoGrant(grant).
		SetIdentity(identity).
		SetName(name).
		SetValidFor(i.TTL).
		ToJWT()
}
