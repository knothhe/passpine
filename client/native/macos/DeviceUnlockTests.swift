import Foundation
import CryptoKit

// Exercises the envelope with software keys; it never prompts for device authentication.
@main
struct DeviceUnlockTests {
    static func main() throws {
        let device = P256.KeyAgreement.PrivateKey()
        let plaintext = Data("fixture private key passphrase".utf8)
        let envelope = try DeviceUnlockEnvelope.seal(plaintext, deviceKey: Data("device fixture".utf8), publicKey: device.publicKey, account: "vault-one")
        guard envelope.version == 2 else { fatalError("New envelope uses a legacy domain") }
        let peer = try P256.KeyAgreement.PublicKey(x963Representation: envelope.ephemeralPublicKey)
        let shared = try device.sharedSecretFromKeyAgreement(with: peer)
        guard try envelope.open(sharedSecret: shared, account: "vault-one") == plaintext else { fatalError("Envelope round trip failed") }
        do {
            _ = try envelope.open(sharedSecret: shared, account: "vault-two")
            fatalError("Envelope accepted a different vault")
        } catch {}
        // Build a pre-rename fixture independently of the current context helper.
        let legacyContext = Data("com.knothhe.passpine.neo.device-unlock.v1\u{0}vault-one".utf8)
        let legacyKey = shared.hkdfDerivedSymmetricKey(using: SHA256.self, salt: envelope.salt, sharedInfo: legacyContext, outputByteCount: 32)
        let legacyBox = try AES.GCM.seal(plaintext, using: legacyKey, authenticating: legacyContext)
        let legacy = DeviceUnlockEnvelope(version: 1, deviceKey: envelope.deviceKey, ephemeralPublicKey: envelope.ephemeralPublicKey, salt: envelope.salt, ciphertext: legacyBox.combined!)
        let decodedLegacy = try JSONDecoder().decode(DeviceUnlockEnvelope.self, from: JSONEncoder().encode(legacy))
        guard try decodedLegacy.open(sharedSecret: shared, account: "vault-one") == plaintext else { fatalError("Legacy envelope no longer opens") }
        do {
            _ = try decodedLegacy.open(sharedSecret: shared, account: "vault-two")
            fatalError("Legacy envelope accepted a different vault")
        } catch {}
        for version in [1, 3] {
            let changedVersion = DeviceUnlockEnvelope(version: version, deviceKey: envelope.deviceKey, ephemeralPublicKey: envelope.ephemeralPublicKey, salt: envelope.salt, ciphertext: envelope.ciphertext)
            do {
                _ = try changedVersion.open(sharedSecret: shared, account: "vault-one")
                fatalError("Envelope accepted a changed or unsupported version")
            } catch {}
        }
        let encoded = try JSONEncoder().encode(envelope)
        guard !String(decoding: encoded, as: UTF8.self).contains("fixture private key passphrase") else { fatalError("Envelope contains plaintext") }
        print("Device envelope encryption and vault binding passed; no biometric prompt was used.")
    }
}
