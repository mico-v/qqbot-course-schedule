package wakeup

import (
	"bytes"
	"encoding/hex"
	"testing"
)

func TestCUIDAndADIDDerivation(t *testing.T) {
	const androidID = "0000000000000000"
	if got, want := CUIDFromAndroidID(androidID), "C77D5D04D94F5F56C8A0A6DC3DBF240A|0"; got != want {
		t.Errorf("CUIDFromAndroidID = %q, want %q", got, want)
	}
	if got, want := ADIDFromAndroidID(androidID), "d58a81f457529caf0e642e7f9d97e447112bd763"; got != want {
		t.Errorf("ADIDFromAndroidID = %q, want %q", got, want)
	}
}

func TestNativeDESEncryptVector(t *testing.T) {
	cipher, err := nativeDESEncrypt([]byte("hello!!"), []byte(signAKey))
	if err != nil {
		t.Fatalf("nativeDESEncrypt: %v", err)
	}
	if got, want := hex.EncodeToString(cipher), "4ded54ea27a44e17"; got != want {
		t.Fatalf("cipher = %s, want %s", got, want)
	}
	plain, err := nativeDESDecrypt(cipher, []byte(signAKey))
	if err != nil {
		t.Fatalf("nativeDESDecrypt: %v", err)
	}
	if string(plain) != "hello!!" {
		t.Fatalf("plain = %q, want hello!!", plain)
	}
}

func TestNativeDESRejectsBadInputs(t *testing.T) {
	if _, err := nativeDESSubkeys([]byte("short")); err == nil {
		t.Error("8-byte key required")
	}
	if _, err := nativeDESDecrypt([]byte{1, 2, 3}, []byte(signAKey)); err == nil {
		t.Error("ciphertext length not a multiple of 8 should fail")
	}

	// Encrypt a raw block whose final byte is 0xFF so decryption sees an
	// out-of-range padding length.
	subkeys, err := nativeDESSubkeys([]byte(signAKey))
	if err != nil {
		t.Fatalf("nativeDESSubkeys: %v", err)
	}
	cipher := nativeDESBlock([]byte{0, 0, 0, 0, 0, 0, 0, 0xFF}, subkeys)
	if _, err := nativeDESDecrypt(cipher, []byte(signAKey)); err == nil {
		t.Error("invalid padding length should fail")
	}
}

func TestNativeHexRoundTrip(t *testing.T) {
	input := []byte{0x00, 0x12, 0xAB, 0xFF}
	if got, want := nativeHexEncode(input), "000004080d050f0f"; got != want {
		t.Fatalf("nativeHexEncode = %s, want %s", got, want)
	}
	decoded, err := nativeHexDecode("000004080d050f0f")
	if err != nil {
		t.Fatalf("nativeHexDecode: %v", err)
	}
	if !bytes.Equal(decoded, input) {
		t.Fatalf("decoded = %x, want %x", decoded, input)
	}
}

func TestRC4Vector(t *testing.T) {
	got, err := rc4Crypt([]byte("hello world"), []byte("secretkey"))
	if err != nil {
		t.Fatalf("rc4Crypt: %v", err)
	}
	if want, _ := hex.DecodeString("cdcdb0702391e2275ac02f"); !bytes.Equal(got, want) {
		t.Fatalf("rc4 = %x, want %x", got, want)
	}
}

func TestNativeGetKeyVector(t *testing.T) {
	got := nativeGetKey("450", "KkkEXzX54B")
	want := "6efdb39863b3aec45d0dbef040d283e06a5a724b106edcdae33c39dfaddfd4d95592ede2ea6e77349e45a85dc0958f5f31cd22941af1d9898bc5ba70303e780c"
	if got != want {
		t.Fatalf("nativeGetKey = %s, want %s", got, want)
	}
}

func TestNativeGetSignVector(t *testing.T) {
	if got, want := nativeGetSign("YWJjZGVm", "KkkEXzX54B"), "ca74ef152827c42d7fa52c6e915bba42"; got != want {
		t.Fatalf("nativeGetSign = %s, want %s", got, want)
	}
}

func TestMakeSignAVector(t *testing.T) {
	client := NewClient(Options{})
	signA, err := client.makeSignA("ABCDEFGHIJ")
	if err != nil {
		t.Fatalf("makeSignA: %v", err)
	}
	want := "0b020b0403030a070c020501060c0a090005010f050e0b0207080901040f03010b0b050a0f0700080102020409000105090c0f00020f00080b0b0b04060e050907010300080b0b06000804070e0f050c0e00090b00060f04010d08070103080306020109030f0007010f0d00050b060e0103060b09020d04050106060c0e0e040d030e000402080e020f09020d03050e0805010a020e04070b030600010c070904010f0f040e010f0407070e05000a05"
	if signA != want {
		t.Fatalf("makeSignA = %s, want %s", signA, want)
	}
}
