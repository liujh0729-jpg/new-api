/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
package relay

import "testing"

func TestTaskErrorFromUpstreamResponsePreservesOpenAIError(t *testing.T) {
	taskErr := taskErrorFromUpstreamResponse(
		[]byte(`{"error":{"code":"InvalidParameter","message":"content[1] video pixel count must be at least 409600","type":"invalid_request_error"},"request_id":"req-seedance-1"}`),
		400,
	)

	if taskErr.Code != "InvalidParameter" {
		t.Fatalf("expected upstream code InvalidParameter, got %q", taskErr.Code)
	}
	if taskErr.Message != "content[1] video pixel count must be at least 409600" {
		t.Fatalf("expected upstream message to be preserved, got %q", taskErr.Message)
	}
	if taskErr.StatusCode != 400 {
		t.Fatalf("expected status code 400, got %d", taskErr.StatusCode)
	}
	details, ok := taskErr.Data.(map[string]any)
	if !ok {
		t.Fatalf("expected structured upstream details, got %#v", taskErr.Data)
	}
	if details["type"] != "invalid_request_error" || details["request_id"] != "req-seedance-1" {
		t.Fatalf("expected upstream type and request id to be preserved, got %#v", details)
	}
}

func TestTaskErrorFromUpstreamResponseFallsBackForUnknownBody(t *testing.T) {
	taskErr := taskErrorFromUpstreamResponse([]byte("upstream unavailable"), 503)

	if taskErr.Code != "fail_to_fetch_task" {
		t.Fatalf("expected fallback code, got %q", taskErr.Code)
	}
	if taskErr.Message != "upstream unavailable" {
		t.Fatalf("expected fallback message, got %q", taskErr.Message)
	}
	if taskErr.StatusCode != 503 {
		t.Fatalf("expected status code 503, got %d", taskErr.StatusCode)
	}
}
