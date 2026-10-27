package create

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"

	"github.com/kodmiq/TripGo-Avito-/internal/model"
)

func hashCreateTrip(in model.CreateTrip) string {
	canonical := fmt.Sprintf("%s|%s|%s|%s|%s|%s|%d",
		in.UserID,
		in.DriverID,
		strconv.FormatFloat(in.StartLat, 'f', -1, 64),
		strconv.FormatFloat(in.StartLon, 'f', -1, 64),
		strconv.FormatFloat(in.EndLat, 'f', -1, 64),
		strconv.FormatFloat(in.EndLon, 'f', -1, 64),
		in.Price,
	)
	sum := sha256.Sum256([]byte(canonical))
	return hex.EncodeToString(sum[:])
}
