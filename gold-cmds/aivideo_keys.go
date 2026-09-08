package goldcmds

// hardcodedAIVideoKeys — 20 Agnes Video API keys used by the .aivideo command.
// Rotation policy is sticky sequential-fill (one active key at a time); on a
// 429 / transient error the key goes into exponential-backoff rest and the
// next available key is used.
var hardcodedAIVideoKeys = []string{
	"sk-90S0eLPjU52QCi50k74EHXc8R8sfXucWcLafz8WawCZ2J22A",
	"sk-hjzuhCN4TgrL6OIFqzbHU13KvOAOeohkk2X4YaWG4i8i1rgQ",
	"sk-iVdHeJqixTzJb5BbuX3GdacT3zsbLkmg8O7rnuHpZCXn3e1T",
	"sk-Ehznnm1EVnaqVt6OnTNhbOSWBKzn6VpGnlyA4VsYZThJwMcP",
	"sk-6TMm3E47JoSF8bxBeKQ6A33BYEyxN4WsGpEZ90duYerMFDAh",
	"sk-91hM0JAm9cGHep68Po1KPOPCsAwLUyDiQyI98rl27ui5Lgv6",
	"sk-SPytzJO4i6T2y9WPy4vkkPYmgoKQ6cJV0LLJKiptGsSeO0IH",
	"sk-7530LO6hh67SqMsg72t31HKZj81gCUOwOu0KsofNjfFjfS4W",
	"sk-vwqXuSpAr9VwmArPVWrOJOquA7SUv0OqDKxv6rGWMYRYIqPH",
	"sk-OMDBSXyozdKUSGhl6kCt4vY6ErqyrSO5hh6VCpmnTnUJ86CL",
	"sk-gkEV5MIDE4jTGW4j0wMqsKI1r0A3vSBTXhXM7CNjhHn63k2g",
	"sk-k93towCSXIpMpt09esXvUbTG7H7OoGvrGEEWcyOrUfpCC0ZE",
	"sk-kJr0QdXJNx5P8gzVsuhpQizD5A1tD5IUOVNCAi0ApIcAePWB",
	"sk-TJHlHcHgiszPJkK1jfumkPptxSpBgkUwo9Sp8H9SHyUga07t",
	"sk-wEwNLNhHueljj5OoEhHLZmDLVKTBiTSb1EHVzJN3zQ4Gk5tA",
	"sk-cEc8vzHYlVCjBVFJLkMkhWkJxipFYoWBLOatkAbdpWuwXEKP",
	"sk-e12k8ZIeSw0Z7jyO5TJYNHJBPbO04s4St9EwezLDPn0ECMmt",
	"sk-DkqWbPNdD2iBXKyVcXRRWHJQsfHnKWkA1GuwnjC2z8SOkBfH",
	"sk-szkNTGIVdUo8JlCwiGtg1BUBeCkCjtll0E1AlSpNRW6uIxJN",
	"sk-NRRsKtT1EKNcv5zKPqZUg7CYwfCNKudzxmB6Y1GbjmQR1IfS",
}
