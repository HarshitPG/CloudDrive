package worker

type GCJob struct {
	ContentID string `json:"contentId"`
	BlobKey   string `json:"blobKey"`
}
