package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/cloudflare/cloudflare-go"
	"github.com/joho/godotenv"
)

type Peer struct {
	ID           string   `json:"ID"`
	DNSName      string   `json:"DNSName"`
	TailscaleIPs []string `json:"TailscaleIPs"`
}

type CurrentTailnet struct {
	MagicDNSSuffix string `json:"MagicDNSSuffix"`
}

type Status struct {
	CurrentTailnet CurrentTailnet  `json:"CurrentTailnet"`
	Peers          map[string]Peer `json:"Peer"`
}

func GetTailscaleStatus() (Status, error) {
	status := Status{}
	// exec tailscale status --json
	cmd := exec.Command("tailscale", "status", "--json")
	out, err := cmd.Output()
	if err != nil {
		return status, err
	}
	// parse json
	json.Unmarshal(out, &status)
	return status, nil
}

func Now() string {
	isoUTC := "2006-01-02T15:04:05Z"
	return time.Now().UTC().Format(isoUTC)
}

func init() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}
}

func main() {
	MAGIC_DOMAIN_SUFFIX := os.Getenv("MAGIC_DOMAIN_SUFFIX")
	CF_ZONE_DOMAIN := os.Getenv("CF_ZONE_DOMAIN")

	api, err := cloudflare.NewWithAPIToken(os.Getenv("CF_API_TOKEN"))
	if err != nil {
		panic(err)
	}
	zoneID, err := api.ZoneIDByName(CF_ZONE_DOMAIN)
	if err != nil {
		panic(err)
	}
	zoneIdentifier := cloudflare.ZoneIdentifier(zoneID)
	fmt.Println("ZoneID: ", zoneID)

	records, _, err := api.ListDNSRecords(context.Background(), zoneIdentifier, cloudflare.ListDNSRecordsParams{Type: "A"})
	if err != nil {
		panic(err)
	}
	magicRecordsOnCF := []cloudflare.DNSRecord{}

	fmt.Println("=== CF A Records ===")
	for _, record := range records {
		fmt.Println("CF A Record: ", record.Name, " ", record.Content)
		if strings.HasSuffix(record.Name, MAGIC_DOMAIN_SUFFIX) {
			magicRecordsOnCF = append(magicRecordsOnCF, record)
		}
	}

	fmt.Println("=== Existing Magic Records ===")
	for _, record := range magicRecordsOnCF {
		fmt.Println("Magic Record: ", record.Name, " ", record.Content)
	}

	status, err := GetTailscaleStatus()
	if err != nil {
		panic(err)
	}

	fmt.Println("=== Tailscale status ===")
	magicDNSSuffix := status.CurrentTailnet.MagicDNSSuffix
	fmt.Println("MagicDNSSuffix: ", magicDNSSuffix)
	fmt.Println(len(status.Peers), "peers")
	fmt.Println("=== Peers ===")

	MagicRecordsToKeep := []cloudflare.DNSRecord{}

	for _, peer := range status.Peers {
		dnsName := strings.ReplaceAll(peer.DNSName, magicDNSSuffix+".", "")
		fmt.Println("<= ID: ", peer.ID, " =>")
		fmt.Println("DNSName: ", peer.DNSName)
		fmt.Println("DNSName (new): ", dnsName)
		fmt.Println("TailscaleIPs: ", strings.Join(peer.TailscaleIPs, "; "))

		ipToUse := peer.TailscaleIPs[0]
		if !strings.HasSuffix(dnsName, ".") {
			panic("dnsName must end with a dot: " + dnsName)
		}
		domainToUse := dnsName + MAGIC_DOMAIN_SUFFIX

		fmt.Println("domainToUse: ", domainToUse, " ipToUse: ", ipToUse)

		// if domainToUse not in existedMagicRecords, create it
		found_at := -1
		for idx, record := range magicRecordsOnCF {
			if record.Name == domainToUse {
				found_at = idx
				break
			}
		}

		if found_at == -1 {
			// create new record
			record, err := api.CreateDNSRecord(context.Background(), zoneIdentifier, cloudflare.CreateDNSRecordParams{
				Name:    domainToUse,
				Type:    "A",
				Content: ipToUse,
				Comment: "Automatically created by magicmagicdns at " + Now(),
			})
			if err != nil {
				panic(err)
			}
			fmt.Println("Created record: ", record.Name, " ", record.Content)
			MagicRecordsToKeep = append(MagicRecordsToKeep, record)
		} else {
			if magicRecordsOnCF[found_at].Content != ipToUse {
				fmt.Println("Updating record: ", magicRecordsOnCF[found_at].Name, " ", magicRecordsOnCF[found_at].Content)
				// update record
				comment := "Automatically updated by magicmagicdns at " + Now()
				record, err := api.UpdateDNSRecord(context.Background(), zoneIdentifier, cloudflare.UpdateDNSRecordParams{
					Name:    domainToUse,
					Type:    "A",
					ID:      magicRecordsOnCF[found_at].ID,
					Content: ipToUse,
					Comment: &comment,
				})
				if err != nil {
					panic(err)
				}
				fmt.Println("Updated record: ", record.Name, " ", record.Content)
			} else {
				fmt.Println("No need to update record")
			}

			MagicRecordsToKeep = append(MagicRecordsToKeep, magicRecordsOnCF[found_at])
		}

	}

	fmt.Println("=== Deleting unused records ===")
	for _, recordOnCF := range magicRecordsOnCF {
		found := false
		for _, recordToKeep := range MagicRecordsToKeep {
			if recordOnCF.Name == recordToKeep.Name {
				found = true
				break
			}
		}

		if !found {
			if !strings.Contains(recordOnCF.Comment, "magicmagicdns") {
				fmt.Println("Skip deleting record: ", recordOnCF.Name, " ", recordOnCF.Content, " because it's not created by magicmagicdns")
				continue
			}

			fmt.Println("Deleting record: ", recordOnCF.Name, " ", recordOnCF.Content)
			err := api.DeleteDNSRecord(context.Background(), zoneIdentifier, recordOnCF.ID)
			if err != nil {
				panic(err)
			}
		}
	}

	fmt.Println("=== Done ===")
}
