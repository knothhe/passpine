import Foundation
import CryptoKit

// Exercises the envelope with software keys; it never prompts for device authentication.
@main
struct DeviceUnlockTests {
    static func main() throws {
        let device = P256.KeyAgreement.PrivateKey()
        let plaintext = Data("fixture private key passphrase".utf8)
        let envelope = try DeviceUnlockEnvelope.seal(plaintext, deviceKey: Data("device fixture".utf8), publicKey: device.publicKey, account: "vault-one")
        let peer = try P256.KeyAgreement.PublicKey(x963Representation: envelope.ephemeralPublicKey)
        let shared = try device.sharedSecretFromKeyAgreement(with: peer)
        guard try envelope.open(sharedSecret: shared, account: "vault-one") == plaintext else { fatalError("Envelope round trip failed") }
        do {
            _ = try envelope.open(sharedSecret: shared, account: "vault-two")
            fatalError("Envelope accepted a different vault")
        } catch {}
        let encoded = try JSONEncoder().encode(envelope)
        guard !String(decoding: encoded, as: UTF8.self).contains("fixture private key passphrase") else { fatalError("Envelope contains plaintext") }
        print("Device envelope encryption and vault binding passed; no biometric prompt was used.")
    }
}
