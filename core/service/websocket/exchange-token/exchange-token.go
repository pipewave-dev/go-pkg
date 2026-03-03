package exchangetoken

import (
	"context"
	"time"

	voAuth "github.com/pipewave-dev/go-pkg/core/domain/value-object/auth"
	"github.com/pipewave-dev/go-pkg/pkg/observer"
	"github.com/pipewave-dev/go-pkg/shared/aerror"
	"github.com/pipewave-dev/go-pkg/shared/utils/fn"
)

const fnNameExchange = "*exchangeToken.Exchange"

func (r *exchangeToken) Exchange(ctx context.Context, auth voAuth.WebsocketAuth) (connTmpToken string, aErr aerror.AError) {
	var op observer.Operation
	ctx, op = r.obs.StartOperation(ctx, fnNameExchange)
	defer op.Finish(aErr)

	connTmpToken = fn.NewNanoID()

	setable := r.store.Set(ctx, connTmpToken, auth.Encode(), tokenTTL*time.Second)
	if !setable {
		return "", aerror.New(ctx, aerror.ErrUnexpectedRedis, nil)
	}

	return connTmpToken, nil
}

const fnNameScanConnToken = "*exchangeToken.ScanConnToken"

func (r *exchangeToken) ScanConnToken(ctx context.Context, connTmpToken string) (auth voAuth.WebsocketAuth, aErr aerror.AError) {
	var op observer.Operation
	ctx, op = r.obs.StartOperation(ctx, fnNameScanConnToken)
	defer op.Finish(aErr)

	authEncoded := ""
	found := r.store.Get(ctx, connTmpToken, &authEncoded)
	if !found {
		return voAuth.WebsocketAuth{}, aerror.New(ctx, aerror.RecordNotFound, nil)
	}

	auth = voAuth.WebsocketAuth{}
	err := auth.Decode([]byte(authEncoded))
	if err != nil {
		return voAuth.WebsocketAuth{}, aerror.New(ctx, aerror.ErrUnexpectedCodeLogic, err)
	}
	return auth, nil
}
