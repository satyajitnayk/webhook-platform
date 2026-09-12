package main

import (
	"fmt"
	"log"
	"strings"
)

func logDelivery(
	delivery Delivery,
	attempt int,
	action string,
	fields ...any,
) {
	parts := []string{
		fmt.Sprintf("delivery_id=%s", delivery.ID),
		fmt.Sprintf("webhook_id=%s", delivery.WebhookID),
		fmt.Sprintf("attempt=%d", attempt),
		fmt.Sprintf("action=%s", action),
	}

	for i := 0; i < len(fields); i += 2 {
		if i+1 >= len(fields) {
			break
		}

		parts = append(
			parts,
			fmt.Sprintf("%v=%v", fields[i], fields[i+1]),
		)
	}

	log.Println(strings.Join(parts, " "))
}
