package alpaca

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/luxfi/broker/pkg/types"
)

// --- DocumentManager implementation ---

func (p *Provider) UploadDocument(ctx context.Context, accountID string, doc *types.DocumentUpload) (*types.Document, error) {
	body := map[string]interface{}{
		"document_type": doc.DocumentType,
		"content":       doc.Content,
		"mime_type":     doc.MimeType,
	}
	if doc.DocumentSubType != "" {
		body["document_sub_type"] = doc.DocumentSubType
	}

	data, _, err := p.do(ctx, http.MethodPost, "/v1/accounts/"+accountID+"/documents/upload", body)
	if err != nil {
		return nil, err
	}
	return p.parseDocument(data)
}

func (p *Provider) ListDocuments(ctx context.Context, accountID string, params *types.DocumentParams) ([]*types.Document, error) {
	path := "/v1/accounts/" + accountID + "/documents"
	sep := "?"
	if params != nil {
		if params.Start != "" {
			path += sep + "start=" + params.Start
			sep = "&"
		}
		if params.End != "" {
			path += sep + "end=" + params.End
		}
	}

	data, _, err := p.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}

	var raw []json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	docs := make([]*types.Document, 0, len(raw))
	for _, r := range raw {
		d, err := p.parseDocument(r)
		if err != nil {
			continue
		}
		docs = append(docs, d)
	}
	return docs, nil
}

func (p *Provider) GetDocument(ctx context.Context, accountID, documentID string) (*types.Document, error) {
	data, _, err := p.do(ctx, http.MethodGet, "/v1/accounts/"+accountID+"/documents/"+documentID, nil)
	if err != nil {
		return nil, err
	}
	return p.parseDocument(data)
}

func (p *Provider) DownloadDocument(ctx context.Context, accountID, documentID string) ([]byte, string, error) {
	data, _, err := p.do(ctx, http.MethodGet, fmt.Sprintf("/v1/accounts/%s/documents/%s/download", accountID, documentID), nil)
	if err != nil {
		return nil, "", err
	}
	// Alpaca returns the raw document bytes; content type is typically application/pdf.
	return data, "application/pdf", nil
}

// GetSubaccountConfirms returns trade confirmations and statements for a subaccount.
func (p *Provider) GetSubaccountConfirms(ctx context.Context, accountID string, params map[string]string) ([]types.Document, error) {
	path := "/v1/accounts/" + accountID + "/documents?type=trade_confirmation"
	for k, v := range params {
		path += "&" + k + "=" + v
	}

	data, _, err := p.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}

	var raw []json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	docs := make([]types.Document, 0, len(raw))
	for _, r := range raw {
		d, err := p.parseDocument(r)
		if err != nil {
			continue
		}
		docs = append(docs, *d)
	}
	return docs, nil
}

func (p *Provider) parseDocument(data []byte) (*types.Document, error) {
	var raw struct {
		ID              string `json:"id"`
		DocumentType    string `json:"document_type"`
		DocumentSubType string `json:"document_sub_type"`
		Name            string `json:"name"`
		Status          string `json:"status"`
		CreatedAt       string `json:"created_at"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	return &types.Document{
		ID:              raw.ID,
		DocumentType:    raw.DocumentType,
		DocumentSubType: raw.DocumentSubType,
		Name:            raw.Name,
		Status:          raw.Status,
		CreatedAt:       raw.CreatedAt,
	}, nil
}
