import Foundation
import CryptoKit
@preconcurrency import LocalAuthentication
import Security

struct Request: Decodable {
    let action: String
    let account: String?
    let secret: Data?
    let envelope: DeviceUnlockEnvelope?
}

struct Response: Encodable {
    var available = false
    var envelope: DeviceUnlockEnvelope?
    var secret: Data?
    var error: String?
}

@main
struct DeviceUnlockMain {
    static func main() async {
        var response = Response()
        let context = LAContext()
        context.localizedFallbackTitle = "使用 Mac 登录密码"
        context.localizedReason = "解锁 Passpine Neo 密码库"
        context.touchIDAuthenticationAllowableReuseDuration = 0
        defer { context.invalidate() }
        do {
            let data = FileHandle.standardInput.readDataToEndOfFile()
            guard data.count <= 1024 * 1024 else { throw UnlockError.message("系统解锁请求过大") }
            let request = try JSONDecoder().decode(Request.self, from: data)
            response.available = SecureEnclave.isAvailable && context.canEvaluatePolicy(.deviceOwnerAuthentication, error: nil)
            if request.action != "status" {
                guard response.available, let account = request.account, !account.isEmpty else {
                    throw UnlockError.message("此设备不支持安全的系统解锁，请使用私钥口令")
                }
                if request.action == "enroll" {
                    guard let secret = request.secret, let passphrase = String(data: secret, encoding: .utf8), !passphrase.isEmpty else {
                        throw UnlockError.message("请输入私钥口令")
                    }
                    try await context.evaluatePolicy(.deviceOwnerAuthentication, localizedReason: "为 Passpine Neo 启用 Touch ID 或 Mac 登录密码解锁")
                    let envelope: DeviceUnlockEnvelope = try await withCheckedThrowingContinuation { continuation in
                        DispatchQueue.global(qos: .userInitiated).async {
                            do {
                                let value = try DeviceUnlockEnvelope.create(passphrase: passphrase, account: account, context: context)
                                guard try value.decrypt(account: account, context: context) == passphrase else { throw UnlockError.message("系统解锁验证失败，未保存解锁数据") }
                                continuation.resume(returning: value)
                            } catch { continuation.resume(throwing: error) }
                        }
                    }
                    response.envelope = envelope
                } else if request.action == "unlock", let envelope = request.envelope {
                    response.secret = try await withCheckedThrowingContinuation { continuation in
                        DispatchQueue.global(qos: .userInitiated).async {
                            do { continuation.resume(returning: Data(try envelope.decrypt(account: account, context: context).utf8)) }
                            catch { continuation.resume(throwing: error) }
                        }
                    }
                } else {
                    throw UnlockError.message("无效的系统解锁请求")
                }
            }
        } catch UnlockError.message(let message) {
            response.error = message
        } catch {
            response.error = "系统验证未完成，请使用私钥口令解锁"
        }
        if let output = try? JSONEncoder().encode(response) {
            FileHandle.standardOutput.write(output)
        }
    }
}
