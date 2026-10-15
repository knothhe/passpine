import Foundation
import CryptoKit
import LocalAuthentication
import Security

enum UnlockError: Error { case message(String) }

/// The key blob is wrapped by this Mac's Secure Enclave, not an exported private key.
struct DeviceUnlockEnvelope: Codable, Sendable {
    let version: Int
    let deviceKey: Data
    let ephemeralPublicKey: Data
    let salt: Data
    let ciphertext: Data

    static func context(account: String) -> Data {
        Data(("com.knothhe.passpine.neo.device-unlock.v1\u{0}" + account).utf8)
    }
    static func seal(_ plaintext: Data, deviceKey: Data, publicKey: P256.KeyAgreement.PublicKey, account: String) throws -> Self {
        let ephemeral = P256.KeyAgreement.PrivateKey()
        let shared = try ephemeral.sharedSecretFromKeyAgreement(with: publicKey)
        let salt = SymmetricKey(size: .bits256).withUnsafeBytes { Data($0) }
        let context = context(account: account)
        let key = shared.hkdfDerivedSymmetricKey(using: SHA256.self, salt: salt, sharedInfo: context, outputByteCount: 32)
        let box = try AES.GCM.seal(plaintext, using: key, authenticating: context)
        guard let combined = box.combined else { throw UnlockError.message("无法创建设备解锁数据。") }
        return Self(version: 1, deviceKey: deviceKey, ephemeralPublicKey: ephemeral.publicKey.x963Representation, salt: salt, ciphertext: combined)
    }
    func open(sharedSecret: SharedSecret, account: String) throws -> Data {
        guard version == 1, salt.count == 32 else { throw UnlockError.message("设备解锁数据已损坏，请使用私钥口令解锁后重新启用。") }
        let context = Self.context(account: account)
        let key = sharedSecret.hkdfDerivedSymmetricKey(using: SHA256.self, salt: salt, sharedInfo: context, outputByteCount: 32)
        return try AES.GCM.open(AES.GCM.SealedBox(combined: ciphertext), using: key, authenticating: context)
    }
    static func create(passphrase: String, account: String, context: LAContext) throws -> Self {
        guard SecureEnclave.isAvailable else { throw UnlockError.message("此 Mac 不支持 Secure Enclave，请使用私钥口令解锁。") }
        guard let access = SecAccessControlCreateWithFlags(nil, kSecAttrAccessibleWhenUnlockedThisDeviceOnly, [.privateKeyUsage, .userPresence], nil) else {
            throw UnlockError.message("无法设置设备密钥的系统验证保护。")
        }
        let device = try SecureEnclave.P256.KeyAgreement.PrivateKey(accessControl: access, authenticationContext: context)
        return try seal(Data(passphrase.utf8), deviceKey: device.dataRepresentation, publicKey: device.publicKey, account: account)
    }
    func decrypt(account: String, context: LAContext) throws -> String {
        guard version == 1 else { throw UnlockError.message("不支持此设备解锁数据版本，请重新启用系统解锁。") }
        let device = try SecureEnclave.P256.KeyAgreement.PrivateKey(dataRepresentation: deviceKey, authenticationContext: context)
        let peer = try P256.KeyAgreement.PublicKey(x963Representation: ephemeralPublicKey)
        // Authentication is enforced by the protected private-key operation itself.
        let shared = try device.sharedSecretFromKeyAgreement(with: peer)
        var plaintext = try open(sharedSecret: shared, account: account)
        defer { plaintext.resetBytes(in: plaintext.startIndex..<plaintext.endIndex) }
        guard let value = String(data: plaintext, encoding: .utf8), !value.isEmpty else {
            throw UnlockError.message("设备解锁数据无效，请使用私钥口令解锁后重新启用。")
        }
        return value
    }
}
