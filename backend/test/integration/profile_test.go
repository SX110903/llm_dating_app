package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func encodeTestPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	return buf.Bytes()
}

func loginAccessToken(t *testing.T, server *httptest.Server, email, password string) string {
	t.Helper()
	resp := doLogin(t, server, email, password)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var body struct {
		AccessToken string `json:"access_token"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	return body.AccessToken
}

func uploadPhoto(t *testing.T, server *httptest.Server, accessToken, filename string, data []byte) *http.Response {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("photo", filename)
	require.NoError(t, err)
	_, err = part.Write(data)
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, server.URL+"/api/v1/profile/photos", &body)
	require.NoError(t, err)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := server.Client().Do(req)
	require.NoError(t, err)
	return resp
}

func authedJSONRequest(t *testing.T, server *httptest.Server, method, path, accessToken string, payload any) *http.Response {
	t.Helper()
	var reader *bytes.Reader
	if payload != nil {
		raw, err := json.Marshal(payload)
		require.NoError(t, err)
		reader = bytes.NewReader(raw)
	} else {
		reader = bytes.NewReader(nil)
	}
	req, err := http.NewRequestWithContext(context.Background(), method, server.URL+path, reader)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := server.Client().Do(req)
	require.NoError(t, err)
	return resp
}

// TestPhotoUploadRejectsFakeMagicBytes proves the server sniffs real content
// instead of trusting the filename: a text file renamed to end in .jpg must
// be rejected.
func TestPhotoUploadRejectsFakeMagicBytes(t *testing.T) {
	pool := startPostgres(t)
	redisClient := startRedis(t)
	server := newTestServer(t, pool, redisClient, false, []string{"http://localhost:5173"})

	email := uniqueEmail()
	registerUser(t, server, email, testPassword)
	accessToken := loginAccessToken(t, server, email, testPassword)

	resp := uploadPhoto(t, server, accessToken, "totally-a-photo.jpg", []byte("this is not an image, just text pretending to be one"))
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode)

	var body struct {
		Code string `json:"code"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	require.Equal(t, "UNSUPPORTED_MIME_TYPE", body.Code)
}

// TestPhotoUploadRejectsOversizedFile proves the 10 MiB limit is enforced.
func TestPhotoUploadRejectsOversizedFile(t *testing.T) {
	pool := startPostgres(t)
	redisClient := startRedis(t)
	server := newTestServer(t, pool, redisClient, false, []string{"http://localhost:5173"})

	email := uniqueEmail()
	registerUser(t, server, email, testPassword)
	accessToken := loginAccessToken(t, server, email, testPassword)

	oversized := bytes.Repeat([]byte("a"), 11*1024*1024)
	resp := uploadPhoto(t, server, accessToken, "huge.png", oversized)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusRequestEntityTooLarge, resp.StatusCode)
}

// TestPhotoLifecycleKeepsExactlyOnePrimaryPhoto exercises upload, listing,
// deletion and the primary-photo consistency rule: deleting the primary
// photo must promote a remaining one.
func TestPhotoLifecycleKeepsExactlyOnePrimaryPhoto(t *testing.T) {
	pool := startPostgres(t)
	redisClient := startRedis(t)
	server := newTestServer(t, pool, redisClient, false, []string{"http://localhost:5173"})

	email := uniqueEmail()
	registerUser(t, server, email, testPassword)
	accessToken := loginAccessToken(t, server, email, testPassword)

	png1 := encodeTestPNG(t)

	firstResp := uploadPhoto(t, server, accessToken, "first.png", png1)
	defer func() { _ = firstResp.Body.Close() }()
	require.Equal(t, http.StatusCreated, firstResp.StatusCode)
	var first struct {
		ID        string `json:"id"`
		IsPrimary bool   `json:"is_primary"`
		CreatedAt string `json:"created_at"`
	}
	require.NoError(t, json.NewDecoder(firstResp.Body).Decode(&first))
	require.True(t, first.IsPrimary)
	createdAt, err := time.Parse(time.RFC3339, first.CreatedAt)
	require.NoError(t, err)
	require.WithinDuration(t, time.Now(), createdAt, time.Minute, "the create response must return the persisted created_at, not a zero value")

	secondResp := uploadPhoto(t, server, accessToken, "second.png", png1)
	defer func() { _ = secondResp.Body.Close() }()
	require.Equal(t, http.StatusCreated, secondResp.StatusCode)
	var second struct {
		ID        string `json:"id"`
		IsPrimary bool   `json:"is_primary"`
	}
	require.NoError(t, json.NewDecoder(secondResp.Body).Decode(&second))
	require.False(t, second.IsPrimary)

	deleteResp := authedJSONRequest(t, server, http.MethodDelete, "/api/v1/profile/photos/"+first.ID, accessToken, nil)
	defer func() { _ = deleteResp.Body.Close() }()
	require.Equal(t, http.StatusNoContent, deleteResp.StatusCode)

	listResp := authedJSONRequest(t, server, http.MethodGet, "/api/v1/profile/photos", accessToken, nil)
	defer func() { _ = listResp.Body.Close() }()
	require.Equal(t, http.StatusOK, listResp.StatusCode)
	var photos []struct {
		ID        string `json:"id"`
		IsPrimary bool   `json:"is_primary"`
	}
	require.NoError(t, json.NewDecoder(listResp.Body).Decode(&photos))
	require.Len(t, photos, 1)
	require.Equal(t, second.ID, photos[0].ID)
	require.True(t, photos[0].IsPrimary, "the remaining photo must have been promoted to primary")
}

// TestGenderPreferenceConsentGatesAndWithdrawalClearsValue is the mandatory
// RGPD consent test: saving genders without consent fails, saving it after
// granting consent succeeds, and withdrawing consent transactionally clears
// the stored value.
func TestGenderPreferenceConsentGatesAndWithdrawalClearsValue(t *testing.T) {
	pool := startPostgres(t)
	redisClient := startRedis(t)
	server := newTestServer(t, pool, redisClient, false, []string{"http://localhost:5173"})

	email := uniqueEmail()
	registerUser(t, server, email, testPassword)
	accessToken := loginAccessToken(t, server, email, testPassword)

	withoutConsent := authedJSONRequest(t, server, http.MethodPut, "/api/v1/profile/preferences", accessToken, map[string]any{
		"min_age": 20, "max_age": 40, "max_distance_km": 50, "genders": []string{"woman"},
	})
	defer func() { _ = withoutConsent.Body.Close() }()
	require.Equal(t, http.StatusUnprocessableEntity, withoutConsent.StatusCode)

	grantResp := authedJSONRequest(t, server, http.MethodPost, "/api/v1/account/consents", accessToken, map[string]any{
		"purpose": "matching_gender_preferences",
	})
	defer func() { _ = grantResp.Body.Close() }()
	require.Equal(t, http.StatusCreated, grantResp.StatusCode)

	withConsent := authedJSONRequest(t, server, http.MethodPut, "/api/v1/profile/preferences", accessToken, map[string]any{
		"min_age": 20, "max_age": 40, "max_distance_km": 50, "genders": []string{"woman", "man"},
	})
	defer func() { _ = withConsent.Body.Close() }()
	require.Equal(t, http.StatusOK, withConsent.StatusCode)
	var saved struct {
		Genders []string `json:"genders"`
	}
	require.NoError(t, json.NewDecoder(withConsent.Body).Decode(&saved))
	require.ElementsMatch(t, []string{"woman", "man"}, saved.Genders)

	withdrawResp := authedJSONRequest(t, server, http.MethodDelete, "/api/v1/account/consents/matching_gender_preferences", accessToken, nil)
	defer func() { _ = withdrawResp.Body.Close() }()
	require.Equal(t, http.StatusNoContent, withdrawResp.StatusCode)

	afterWithdraw := authedJSONRequest(t, server, http.MethodGet, "/api/v1/profile/preferences", accessToken, nil)
	defer func() { _ = afterWithdraw.Body.Close() }()
	require.Equal(t, http.StatusOK, afterWithdraw.StatusCode)
	var current struct {
		Genders []string `json:"genders"`
	}
	require.NoError(t, json.NewDecoder(afterWithdraw.Body).Decode(&current))
	require.Empty(t, current.Genders, "withdrawing consent must clear genders transactionally")
}
