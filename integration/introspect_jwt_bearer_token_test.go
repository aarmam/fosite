/*
 * Copyright © 2015-2018 Aeneas Rekkas <aeneas+oss@aeneas.io>
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 * @author		Aeneas Rekkas <aeneas+oss@aeneas.io>
 * @copyright 	2015-2018 Aeneas Rekkas <aeneas+oss@aeneas.io>
 * @license 	Apache-2.0
 *
 */

package integration_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"

	"github.com/ory/fosite"
	"github.com/ory/fosite/compose"
	"github.com/ory/fosite/integration/clients"
)

type introspectJwtBearerTokenSuite struct {
	suite.Suite

	clientJWT          *clients.JWTBearer
	clientIntrospect   *clients.Introspect
	clientTokenPayload *clients.JWTBearerPayload
	appTokenPayload    *clients.JWTBearerPayload

	scopes   []string
	audience []string
}

func (s *introspectJwtBearerTokenSuite) SetupTest() {
	s.scopes = []string{"fosite"}
	s.audience = []string{"https://www.ory.sh/api", "https://vk.com"}

	s.clientTokenPayload = &clients.JWTBearerPayload{
		Issuer:   firstJWTBearerIssuer,
		Subject:  firstJWTBearerSubject,
		Audience: s.audience,
		Expires:  time.Now().Add(time.Hour).Unix(),
	}

	s.appTokenPayload = &clients.JWTBearerPayload{
		Issuer:   secondJWTBearerIssuer,
		Subject:  secondJWTBearerSubject,
		Audience: []string{"https://www.ory.sh/api"},
		Expires:  time.Now().Add(time.Hour).Unix(),
	}
}

func (s *introspectJwtBearerTokenSuite) getJWTClient() *clients.JWTBearer {
	client := *s.clientJWT
	client.SetPrivateKey(firstKeyID, firstPrivateKey)

	return &client
}

func (s *introspectJwtBearerTokenSuite) getAPPJWTClient() *clients.JWTBearer {
	client := *s.clientJWT
	client.SetPrivateKey(secondKeyID, secondPrivateKey)

	return &client
}

func (s *introspectJwtBearerTokenSuite) assertSuccessResponse(
	t *testing.T,
	response *clients.IntrospectResponse,
	err error,
) {
	assert.Nil(t, err)
	assert.NotNil(t, response)

	assert.True(t, response.Active)
	assert.Equal(t, response.Subject, firstJWTBearerSubject)
	assert.NotEmpty(t, response.ExpiresAt)
	assert.NotEmpty(t, response.IssuedAt)
	assert.Equal(t, time.Unix(response.ExpiresAt, 0).Sub(time.Unix(response.IssuedAt, 0)), time.Hour)
	assert.Equal(t, response.Audience, s.audience)
}

func (s *introspectJwtBearerTokenSuite) assertUnauthorizedResponse(
	t *testing.T,
	response *clients.IntrospectResponse,
	err error,
) {
	assert.Nil(t, response)
	assert.NotNil(t, err)

	retrieveError, ok := err.(*clients.RequestError)
	assert.True(t, ok)
	assert.Equal(t, retrieveError.Response.StatusCode, http.StatusUnauthorized)
}

func (s *introspectJwtBearerTokenSuite) TestBaseConfiguredClient() {
	ctx := context.Background()

	scopes := []string{"fosite", "docker"}
	token, _ := s.getJWTClient().GetToken(ctx, s.clientTokenPayload, scopes)
	token2, _ := s.getAPPJWTClient().GetToken(ctx, s.appTokenPayload, s.scopes)

	response, err := s.clientIntrospect.IntrospectToken(
		ctx,
		clients.IntrospectForm{
			Token:  token.AccessToken,
			Scopes: nil,
		},
		map[string]string{"Authorization": "bearer " + token2.AccessToken},
	)

	s.assertSuccessResponse(s.T(), response, err)
	assert.NotEmpty(s.T(), response.Scope, scopes)
}

func (s *introspectJwtBearerTokenSuite) TestInvalidScopes() {
	ctx := context.Background()

	token, _ := s.getJWTClient().GetToken(ctx, s.clientTokenPayload, s.scopes)
	token2, _ := s.getAPPJWTClient().GetToken(ctx, s.appTokenPayload, s.scopes)

	response, err := s.clientIntrospect.IntrospectToken(
		ctx,
		clients.IntrospectForm{
			Token:  token.AccessToken,
			Scopes: []string{"invalid"},
		},
		map[string]string{"Authorization": "bearer " + token2.AccessToken},
	)

	assert.Nil(s.T(), err)
	assert.NotNil(s.T(), response)
	assert.False(s.T(), response.Active)
}

func (s *introspectJwtBearerTokenSuite) TestIntrospectByYourself() {
	ctx := context.Background()
	token, _ := s.getJWTClient().GetToken(ctx, s.clientTokenPayload, s.scopes)

	response, err := s.clientIntrospect.IntrospectToken(
		ctx,
		clients.IntrospectForm{
			Token:  token.AccessToken,
			Scopes: nil,
		},
		map[string]string{"Authorization": "bearer " + token.AccessToken},
	)

	s.assertUnauthorizedResponse(s.T(), response, err)
}

func TestIntrospectJwtBearerTokenSuite(t *testing.T) {
	provider := compose.Compose(
		&compose.Config{
			JWTSkipClientAuth:     true,
			JWTIDOptional:         true,
			JWTIssuedDateOptional: true,
			AccessTokenLifespan:   time.Hour,
			TokenURL:              "https://www.ory.sh/api",
		},
		fositeStore,
		jwtStrategy,
		nil,
		compose.OAuth2ClientCredentialsGrantFactory,
		compose.OAuth2AuthorizeJWTGrantFactory,
		compose.OAuth2TokenIntrospectionFactory,
	)
	testServer := mockServer(t, provider, &fosite.DefaultSession{})
	defer testServer.Close()

	suite.Run(t, &introspectJwtBearerTokenSuite{
		clientJWT:        newJWTBearerAppClient(testServer),
		clientIntrospect: clients.NewIntrospectClient(testServer.URL + "/introspect"),
	})
}