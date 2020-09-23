package client

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"

	"gitlab.com/scpcorp/ScPrime/modules"
	"gitlab.com/scpcorp/ScPrime/node/api"
	"gitlab.com/scpcorp/ScPrime/pubaccesskey"

	"gitlab.com/NebulousLabs/errors"
)

// RenterSkyfileGet wraps RenterFileRootGet to query a pubfile.
func (c *Client) RenterSkyfileGet(siaPath modules.SiaPath, root bool) (rf api.RenterFile, err error) {
	if !root {
		siaPath, err = modules.SkynetFolder.Join(siaPath.String())
		if err != nil {
			return
		}
	}
	return c.RenterFileRootGet(siaPath)
}

// SkynetPublinkGet uses the /pubaccess/publink endpoint to download a publink
// file.
func (c *Client) SkynetPublinkGet(publink string) ([]byte, modules.PubfileMetadata, error) {
	return c.SkynetPublinkGetWithTimeout(publink, -1)
}

// SkynetPublinkGetWithTimeout uses the /pubaccess/publink endpoint to download a
// pubaccess file, specifying the given timeout.
func (c *Client) SkynetPublinkGetWithTimeout(publink string, timeout int) ([]byte, modules.PubfileMetadata, error) {
	params := make(map[string]string)
	// Only set the timeout if it's valid. Seeing as 0 is a valid timeout,
	// callers need to pass -1 to ignore it.
	if timeout >= 0 {
		params["timeout"] = fmt.Sprintf("%d", timeout)
	}
	return c.skynetSkylinkGetWithParameters(publink, params)
}

// skynetSkylinkGetWithParameters uses the /pubaccess/publink endpoint to download
// a publink file, specifying the given parameters.
// The caller of this function is responsible for validating the parameters!
func (c *Client) skynetSkylinkGetWithParameters(publink string, params map[string]string) ([]byte, modules.PubfileMetadata, error) {
	values := url.Values{}
	for k, v := range params {
		values.Set(k, v)
	}

	getQuery := fmt.Sprintf("/pubaccess/publink/%s?%s", publink, values.Encode())
	header, fileData, err := c.getRawResponse(getQuery)
	if err != nil {
		return nil, modules.PubfileMetadata{}, errors.AddContext(err, "error fetching api response")
	}

	var sm modules.PubfileMetadata
	strMetadata := header.Get("Pubaccess-File-Metadata")
	if strMetadata != "" {
		err = json.Unmarshal([]byte(strMetadata), &sm)
		if err != nil {
			return nil, modules.PubfileMetadata{}, errors.AddContext(err, "unable to unmarshal pubfile metadata")
		}
	}
	return fileData, sm, errors.AddContext(err, "unable to fetch publink data")
}

// SkynetPublinkHead uses the /pubaccess/publink endpoint to get the headers that
// are returned if the pubfile were to be requested using the SkynetPublinkGet
// method.
func (c *Client) SkynetPublinkHead(publink string) (int, http.Header, error) {
	return c.SkynetPublinkHeadWithParameters(publink, url.Values{})
}

// SkynetPublinkHeadWithTimeout uses the /pubaccess/publink endpoint to get the
// headers that are returned if the pubfile were to be requested using the
// SkynetPublinkGet method. It allows to pass a timeout parameter for the
// request.
func (c *Client) SkynetPublinkHeadWithTimeout(publink string, timeout int) (int, http.Header, error) {
	values := url.Values{}
	values.Set("timeout", fmt.Sprintf("%d", timeout))
	return c.SkynetPublinkHeadWithParameters(publink, values)
}

// SkynetPublinkHeadWithAttachment uses the /pubaccess/publink endpoint to get the
// headers that are returned if the pubfile were to be requested using the
// SkynetPublinkGet method. It allows to pass the 'attachment' parameter.
func (c *Client) SkynetPublinkHeadWithAttachment(publink string, attachment bool) (int, http.Header, error) {
	values := url.Values{}
	values.Set("attachment", fmt.Sprintf("%t", attachment))
	return c.SkynetPublinkHeadWithParameters(publink, values)
}

// SkynetPublinkHeadWithFormat uses the /pubaccess/publink endpoint to get the
// headers that are returned if the pubfile were to be requested using the
// SkynetPublinkGet method. It allows to pass the 'format' parameter.
func (c *Client) SkynetPublinkHeadWithFormat(publink string, format modules.PubfileFormat) (int, http.Header, error) {
	values := url.Values{}
	values.Set("format", string(format))
	return c.SkynetPublinkHeadWithParameters(publink, values)
}

// SkynetPublinkHeadWithParameters uses the /pubaccess/publink endpoint to get the
// headers that are returned if the pubfile were to be requested using the
// SkynetPublinkGet method. The values are encoded in the querystring.
func (c *Client) SkynetPublinkHeadWithParameters(publink string, values url.Values) (int, http.Header, error) {
	getQuery := fmt.Sprintf("/pubaccess/publink/%s?%s", publink, values.Encode())
	return c.head(getQuery)
}

// SkynetPublinkConcatGet uses the /pubaccess/publink endpoint to download a
// publink file with the 'concat' format specified.
func (c *Client) SkynetPublinkConcatGet(publink string) ([]byte, modules.PubfileMetadata, error) {
	values := url.Values{}
	values.Set("format", string(modules.SkyfileFormatConcat))
	getQuery := fmt.Sprintf("/pubaccess/publink/%s?%s", publink, values.Encode())
	var reader io.Reader
	header, body, err := c.getReaderResponse(getQuery)
	if err != nil {
		return nil, modules.PubfileMetadata{}, errors.AddContext(err, "error fetching api response")
	}
	defer body.Close()
	reader = body

	// Read the fileData.
	fileData, err := ioutil.ReadAll(reader)
	if err != nil {
		return nil, modules.PubfileMetadata{}, err
	}

	var sm modules.PubfileMetadata
	strMetadata := header.Get("Pubaccess-File-Metadata")
	if strMetadata != "" {
		err = json.Unmarshal([]byte(strMetadata), &sm)
		if err != nil {
			return nil, modules.PubfileMetadata{}, errors.AddContext(err, "unable to unmarshal pubfile metadata")
		}
	}
	return fileData, sm, errors.AddContext(err, "unable to fetch publink data")
}

// SkynetPublinkReaderGet uses the /pubaccess/publink endpoint to fetch a reader of
// the file data.
func (c *Client) SkynetPublinkReaderGet(publink string) (io.ReadCloser, error) {
	getQuery := fmt.Sprintf("/pubaccess/publink/%s", publink)
	_, reader, err := c.getReaderResponse(getQuery)
	return reader, errors.AddContext(err, "unable to fetch publink data")
}

// SkynetPublinkConcatReaderGet uses the /pubaccess/publink endpoint to fetch a
// reader of the file data with the 'concat' format specified.
func (c *Client) SkynetPublinkConcatReaderGet(publink string) (io.ReadCloser, error) {
	values := url.Values{}
	values.Set("format", string(modules.SkyfileFormatConcat))
	getQuery := fmt.Sprintf("/pubaccess/publink/%s?%s", publink, values.Encode())
	_, reader, err := c.getReaderResponse(getQuery)
	return reader, errors.AddContext(err, "unable to fetch publink data")
}

// SkynetPublinkTarReaderGet uses the /pubaccess/publink endpoint to fetch a
// reader of the file data with the 'tar' format specified.
func (c *Client) SkynetPublinkTarReaderGet(publink string) (http.Header, io.ReadCloser, error) {
	values := url.Values{}
	values.Set("format", string(modules.SkyfileFormatTar))
	getQuery := fmt.Sprintf("/pubaccess/publink/%s?%s", publink, values.Encode())
	header, reader, err := c.getReaderResponse(getQuery)
	return header, reader, errors.AddContext(err, "unable to fetch publink data")
}

// SkynetPublinkTarGzReaderGet uses the /pubaccess/publink endpoint to fetch a
// reader of the file data with the 'targz' format specified.
func (c *Client) SkynetPublinkTarGzReaderGet(publink string) (http.Header, io.ReadCloser, error) {
	values := url.Values{}
	values.Set("format", string(modules.SkyfileFormatTarGz))
	getQuery := fmt.Sprintf("/pubaccess/publink/%s?%s", publink, values.Encode())
	header, reader, err := c.getReaderResponse(getQuery)
	return header, reader, errors.AddContext(err, "unable to fetch publink data")
}

// SkynetPublinkZipReaderGet uses the /pubaccess/publink endpoint to fetch a
// reader of the file data with the 'zip' format specified.
func (c *Client) SkynetPublinkZipReaderGet(publink string) (http.Header, io.ReadCloser, error) {
	values := url.Values{}
	values.Set("format", string(modules.SkyfileFormatZip))
	getQuery := fmt.Sprintf("/pubaccess/publink/%s?%s", publink, values.Encode())
	header, reader, err := c.getReaderResponse(getQuery)
	return header, reader, errors.AddContext(err, "unable to fetch publink data")
}

// SkynetPublinkPinPost uses the /pubaccess/pin endpoint to pin the file at the
// given publink.
func (c *Client) SkynetPublinkPinPost(publink string, params modules.SkyfilePinParameters) error {
	return c.SkynetPublinkPinPostWithTimeout(publink, params, -1)
}

// SkynetPublinkPinPostWithTimeout uses the /pubaccess/pin endpoint to pin the file
// at the given publink, specifying the given timeout.
func (c *Client) SkynetPublinkPinPostWithTimeout(publink string, params modules.SkyfilePinParameters, timeout int) error {
	// Set the url values.
	values := url.Values{}
	forceStr := fmt.Sprintf("%t", params.Force)
	values.Set("force", forceStr)
	redundancyStr := fmt.Sprintf("%v", params.BaseChunkRedundancy)
	values.Set("basechunkredundancy", redundancyStr)
	rootStr := fmt.Sprintf("%t", params.Root)
	values.Set("root", rootStr)
	values.Set("siapath", params.SiaPath.String())
	values.Set("timeout", fmt.Sprintf("%d", timeout))

	query := fmt.Sprintf("/pubaccess/pin/%s?%s", publink, values.Encode())
	_, _, err := c.postRawResponse(query, nil)
	if err != nil {
		return errors.AddContext(err, "post call to "+query+" failed")
	}
	return nil
}

// SkynetSkyfilePost uses the /pubaccess/pubfile endpoint to upload a pubfile.  The
// resulting publink is returned along with an error.
func (c *Client) SkynetSkyfilePost(params modules.PubfileUploadParameters) (string, api.SkynetSkyfileHandlerPOST, error) {
	// Set the url values.
	values := url.Values{}
	values.Set("filename", params.FileMetadata.Filename)
	dryRunStr := fmt.Sprintf("%t", params.DryRun)
	values.Set("dryrun", dryRunStr)
	forceStr := fmt.Sprintf("%t", params.Force)
	values.Set("force", forceStr)
	modeStr := fmt.Sprintf("%o", params.FileMetadata.Mode)
	values.Set("mode", modeStr)
	redundancyStr := fmt.Sprintf("%v", params.BaseChunkRedundancy)
	values.Set("basechunkredundancy", redundancyStr)
	rootStr := fmt.Sprintf("%t", params.Root)
	values.Set("root", rootStr)

	// Encode SkykeyName or PubaccesskeyID.
	if params.SkykeyName != "" {
		values.Set("pubaccesskeyname", params.SkykeyName)
	}
	hasSkykeyID := params.PubaccesskeyID != pubaccesskey.PubaccesskeyID{}
	if hasSkykeyID {
		values.Set("pubaccesskeyid", params.PubaccesskeyID.ToString())
	}

	// Make the call to upload the file.
	query := fmt.Sprintf("/pubaccess/pubfile/%s?%s", params.SiaPath.String(), values.Encode())
	_, resp, err := c.postRawResponse(query, params.Reader)
	if err != nil {
		return "", api.SkynetSkyfileHandlerPOST{}, errors.AddContext(err, "post call to "+query+" failed")
	}

	// Parse the response to get the publink.
	var rshp api.SkynetSkyfileHandlerPOST
	err = json.Unmarshal(resp, &rshp)
	if err != nil {
		return "", api.SkynetSkyfileHandlerPOST{}, errors.AddContext(err, "unable to parse the publink upload response")
	}
	return rshp.Publink, rshp, err
}

// SkynetSkyfilePostDisableForce uses the /pubaccess/pubfile endpoint to upload a
// pubfile. This method allows to set the Disable-Force header. The resulting
// publink is returned along with an error.
func (c *Client) SkynetSkyfilePostDisableForce(params modules.PubfileUploadParameters, disableForce bool) (string, api.SkynetSkyfileHandlerPOST, error) {
	// Set the url values.
	values := url.Values{}
	values.Set("filename", params.FileMetadata.Filename)
	forceStr := fmt.Sprintf("%t", params.Force)
	values.Set("force", forceStr)
	modeStr := fmt.Sprintf("%o", params.FileMetadata.Mode)
	values.Set("mode", modeStr)
	redundancyStr := fmt.Sprintf("%v", params.BaseChunkRedundancy)
	values.Set("basechunkredundancy", redundancyStr)
	rootStr := fmt.Sprintf("%t", params.Root)
	values.Set("root", rootStr)

	// Set the headers
	headers := map[string]string{"Content-Type": "application/x-www-form-urlencoded"}
	if disableForce {
		headers["Pubaccess-Disable-Force"] = strconv.FormatBool(disableForce)
	}

	// Make the call to upload the file.
	query := fmt.Sprintf("/pubaccess/pubfile/%s?%s", params.SiaPath.String(), values.Encode())
	_, resp, err := c.postRawResponseWithHeaders(query, params.Reader, headers)
	if err != nil {
		return "", api.SkynetSkyfileHandlerPOST{}, errors.AddContext(err, "post call to "+query+" failed")
	}

	// Parse the response to get the publink.
	var rshp api.SkynetSkyfileHandlerPOST
	err = json.Unmarshal(resp, &rshp)
	if err != nil {
		return "", api.SkynetSkyfileHandlerPOST{}, errors.AddContext(err, "unable to parse the publink upload response")
	}
	return rshp.Publink, rshp, err
}

// SkynetSkyfileMultiPartPost uses the /pubaccess/pubfile endpoint to upload a
// pubfile using multipart form data.  The resulting publink is returned along
// with an error.
func (c *Client) SkynetSkyfileMultiPartPost(params modules.SkyfileMultipartUploadParameters) (string, api.SkynetSkyfileHandlerPOST, error) {
	// Set the url values.
	values := url.Values{}
	values.Set("filename", params.Filename)
	values.Set(modules.SkyfileDisableDefaultPathParamName, strconv.FormatBool(params.DisableDefaultPath))
	values.Set(modules.SkyfileDefaultPathParamName, params.DefaultPath)
	forceStr := fmt.Sprintf("%t", params.Force)
	values.Set("force", forceStr)
	redundancyStr := fmt.Sprintf("%v", params.BaseChunkRedundancy)
	values.Set("basechunkredundancy", redundancyStr)
	rootStr := fmt.Sprintf("%t", params.Root)
	values.Set("root", rootStr)

	// Make the call to upload the file.
	query := fmt.Sprintf("/pubaccess/pubfile/%s?%s", params.SiaPath.String(), values.Encode())

	headers := map[string]string{"Content-Type": params.ContentType}
	_, resp, err := c.postRawResponseWithHeaders(query, params.Reader, headers)
	if err != nil {
		return "", api.SkynetSkyfileHandlerPOST{}, errors.AddContext(err, "post call to "+query+" failed")
	}

	// Parse the response to get the publink.
	var rshp api.SkynetSkyfileHandlerPOST
	err = json.Unmarshal(resp, &rshp)
	if err != nil {
		return "", api.SkynetSkyfileHandlerPOST{}, errors.AddContext(err, "unable to parse the publink upload response")
	}
	return rshp.Publink, rshp, err
}

// SkynetConvertSiafileToSkyfilePost uses the /pubaccess/pubfile endpoint to
// convert an existing siafile to a pubfile. The input SiaPath 'convert' is the
// siapath of the siafile that should be converted. The siapath provided inside
// of the upload params is the name that will be used for the base sector of the
// pubfile.
func (c *Client) SkynetConvertSiafileToSkyfilePost(lup modules.PubfileUploadParameters, convert modules.SiaPath) (string, error) {
	// Set the url values.
	values := url.Values{}
	values.Set("filename", lup.FileMetadata.Filename)
	forceStr := fmt.Sprintf("%t", lup.Force)
	values.Set("force", forceStr)
	modeStr := fmt.Sprintf("%o", lup.FileMetadata.Mode)
	values.Set("mode", modeStr)
	redundancyStr := fmt.Sprintf("%v", lup.BaseChunkRedundancy)
	values.Set("redundancy", redundancyStr)
	values.Set("convertpath", convert.String())

	// Make the call to upload the file.
	query := fmt.Sprintf("/pubaccess/pubfile/%s?%s", lup.SiaPath.String(), values.Encode())
	_, resp, err := c.postRawResponse(query, lup.Reader)
	if err != nil {
		return "", errors.AddContext(err, "post call to "+query+" failed")
	}

	// Parse the response to get the publink.
	var rshp api.SkynetSkyfileHandlerPOST
	err = json.Unmarshal(resp, &rshp)
	if err != nil {
		return "", errors.AddContext(err, "unable to parse the publink upload response")
	}
	return rshp.Publink, err
}

// SkynetBlacklistGet requests the /pubaccess/blacklist Get endpoint
func (c *Client) SkynetBlacklistGet() (blacklist api.SkynetBlacklistGET, err error) {
	err = c.get("/pubaccess/blacklist", &blacklist)
	return
}

// SkynetBlacklistPost requests the /pubaccess/blacklist Post endpoint
func (c *Client) SkynetBlacklistPost(additions, removals []string) (err error) {
	sbp := api.SkynetBlacklistPOST{
		Add:    additions,
		Remove: removals,
	}
	data, err := json.Marshal(sbp)
	if err != nil {
		return err
	}
	err = c.post("/pubaccess/blacklist", string(data), nil)
	return
}

// SkynetPortalsGet requests the /pubaccess/portals Get endpoint.
func (c *Client) SkynetPortalsGet() (portals api.SkynetPortalsGET, err error) {
	err = c.get("/pubaccess/portals", &portals)
	return
}

// SkynetPortalsPost requests the /pubaccess/portals Post endpoint.
func (c *Client) SkynetPortalsPost(additions []modules.SkynetPortal, removals []modules.NetAddress) (err error) {
	spp := api.SkynetPortalsPOST{
		Add:    additions,
		Remove: removals,
	}
	data, err := json.Marshal(spp)
	if err != nil {
		return err
	}
	err = c.post("/pubaccess/portals", string(data), nil)
	return
}

// SkynetStatsGet requests the /pubaccess/stats Get endpoint
func (c *Client) SkynetStatsGet() (stats api.SkynetStatsGET, err error) {
	err = c.get("/pubaccess/stats", &stats)
	return
}

// SkykeyGetByName requests the /pubaccess/pubaccesskey Get endpoint using the key name.
func (c *Client) SkykeyGetByName(name string) (pubaccesskey.Pubaccesskey, error) {
	values := url.Values{}
	values.Set("name", name)
	getQuery := fmt.Sprintf("/pubaccess/pubaccesskey?%s", values.Encode())

	var pubaccesskeyGet api.SkykeyGET
	err := c.get(getQuery, &pubaccesskeyGet)
	if err != nil {
		return pubaccesskey.Pubaccesskey{}, err
	}

	var sk pubaccesskey.Pubaccesskey
	err = sk.FromString(pubaccesskeyGet.Pubaccesskey)
	if err != nil {
		return pubaccesskey.Pubaccesskey{}, err
	}

	return sk, nil
}

// SkykeyGetByID requests the /pubaccess/pubaccesskey Get endpoint using the key ID.
func (c *Client) SkykeyGetByID(id pubaccesskey.PubaccesskeyID) (pubaccesskey.Pubaccesskey, error) {
	values := url.Values{}
	values.Set("id", id.ToString())
	getQuery := fmt.Sprintf("/pubaccess/pubaccesskey?%s", values.Encode())

	var pubaccesskeyGet api.SkykeyGET
	err := c.get(getQuery, &pubaccesskeyGet)
	if err != nil {
		return pubaccesskey.Pubaccesskey{}, err
	}

	var sk pubaccesskey.Pubaccesskey
	err = sk.FromString(pubaccesskeyGet.Pubaccesskey)
	if err != nil {
		return pubaccesskey.Pubaccesskey{}, err
	}

	return sk, nil
}

// SkykeyDeleteByIDPost requests the /pubaccess/deletepubaccesskey POST endpoint using the key ID.
func (c *Client) SkykeyDeleteByIDPost(id pubaccesskey.PubaccesskeyID) error {
	values := url.Values{}
	values.Set("id", id.ToString())
	return c.post("/pubaccess/deletepubaccesskey", values.Encode(), nil)
}

// SkykeyDeleteByNamePost requests the /pubaccess/deletepubaccesskey POST endpoint using
// the key name.
func (c *Client) SkykeyDeleteByNamePost(name string) error {
	values := url.Values{}
	values.Set("name", name)
	return c.post("/pubaccess/deletepubaccesskey", values.Encode(), nil)
}

// SkykeyCreateKeyPost requests the /pubaccess/createpubaccesskey POST endpoint.
func (c *Client) SkykeyCreateKeyPost(name string, skType pubaccesskey.PubaccesskeyType) (pubaccesskey.Pubaccesskey, error) {
	// Set the url values.
	values := url.Values{}
	values.Set("name", name)
	values.Set("type", skType.ToString())

	var pubaccesskeyGet api.SkykeyGET
	err := c.post("/pubaccess/createpubaccesskey", values.Encode(), &pubaccesskeyGet)
	if err != nil {
		return pubaccesskey.Pubaccesskey{}, errors.AddContext(err, "createpubaccesskey POST request failed")
	}

	var sk pubaccesskey.Pubaccesskey
	err = sk.FromString(pubaccesskeyGet.Pubaccesskey)
	if err != nil {
		return pubaccesskey.Pubaccesskey{}, errors.AddContext(err, "failed to decode pubaccesskey string")
	}
	return sk, nil
}

// SkykeyAddKeyPost requests the /pubaccess/addpubaccesskey POST endpoint.
func (c *Client) SkykeyAddKeyPost(sk pubaccesskey.Pubaccesskey) error {
	values := url.Values{}
	skString, err := sk.ToString()
	if err != nil {
		return errors.AddContext(err, "failed to encode pubaccesskey as string")
	}
	values.Set("pubaccesskey", skString)

	err = c.post("/pubaccess/addpubaccesskey", values.Encode(), nil)
	if err != nil {
		return errors.AddContext(err, "addpubaccesskey POST request failed")
	}

	return nil
}

// SkykeySkykeysGet requests the /pubaccess/pubaccesskeys GET endpoint.
func (c *Client) SkykeySkykeysGet() ([]pubaccesskey.Pubaccesskey, error) {
	var pubaccesskeysGet api.SkykeysGET
	err := c.get("/pubaccess/pubaccesskeys", &pubaccesskeysGet)
	if err != nil {
		return nil, errors.AddContext(err, "allpubaccesskeys GET request failed")
	}

	res := make([]pubaccesskey.Pubaccesskey, len(pubaccesskeysGet.Pubaccesskeys))
	for i, skGET := range pubaccesskeysGet.Pubaccesskeys {
		err = res[i].FromString(skGET.Pubaccesskey)
		if err != nil {
			return nil, errors.AddContext(err, "failed to decode pubaccesskey string")
		}
	}
	return res, nil
}

// SkynetPublinkGetWithRedirect uses the /pubaccess/publink endpoint to download a
// publink file, specifying whether redirecting is allowed or not.
func (c *Client) SkynetPublinkGetWithRedirect(publink string, allowRedirect bool) ([]byte, modules.PubfileMetadata, error) {
	params := make(map[string]string)
	params["redirect"] = fmt.Sprintf("%t", allowRedirect)
	return c.skynetSkylinkGetWithParameters(publink, params)
}
