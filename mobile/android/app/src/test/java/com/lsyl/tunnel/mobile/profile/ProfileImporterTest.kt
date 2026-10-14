package com.lsyl.tunnel.mobile.profile

import org.json.JSONObject
import org.junit.Assert.assertTrue
import org.junit.Assert.fail
import org.junit.Test
import java.io.ByteArrayOutputStream
import java.util.zip.ZipEntry
import java.util.zip.ZipOutputStream

class ProfileImporterTest {
    @Test
    fun rejectsLoopbackAliasesWithFieldAndCanonicalAddress() {
        listOf("localhost", "LOCALHOST.", "::1", "127.0.0.2", "::ffff:127.0.0.1").forEach { host ->
            val endpoint = if (':' in host) "[$host]:3443" else "$host:3443"
            assertImportRejected(validProfile().put("server_addr", endpoint), "server_addr 不再支持回环地址")
            val tlsProfile = validProfile().apply { getJSONObject("tls").put("server_name", host) }
            assertImportRejected(tlsProfile, "tls.server_name 不再支持回环地址")
            val targetProfile = validProfile().apply {
                getJSONArray("forwards").getJSONObject(0).put("server_target", endpoint)
            }
            assertImportRejected(targetProfile, "server_target 不再支持回环地址")
            val listenProfile = validProfile().apply {
                getJSONArray("forwards").getJSONObject(0).put("listen_addr", endpoint)
            }
            assertImportRejected(listenProfile, "本地监听只能使用 127.0.0.1")
        }
    }

    @Test
    fun allowsCanonicalLoopbackAndOrdinaryDomainsWithoutResolution() {
        listOf("127.0.0.1", "vpn.example.test", "192.168.1.7", "2001:db8::1").forEach {
            requireCanonicalLoopbackHost(it, "server_addr")
        }
    }

    @Test
    fun rejectsProfilesContainingPlaintextPasswordFields() {
        val profile = validProfile().put("password", "secret")

        assertImportRejected(profile, "移动端 Profile 不允许包含 password")
    }

    @Test
    fun rejectsReverseForwardProfilesBeforeOpeningNetworkState() {
        val profile = validProfile().apply {
            getJSONArray("forwards").getJSONObject(0).put("direction", "server_to_client")
        }

        assertImportRejected(profile, "移动端仅支持正向代理")
    }

    private fun assertImportRejected(profile: JSONObject, expectedMessage: String) {
        try {
            ProfileImporter.importFromZip(profileZip(profile))
            fail("expected profile import to fail")
        } catch (err: ProfileImportException) {
            assertTrue("unexpected message: ${err.message}", err.message?.contains(expectedMessage) == true)
        }
    }

    private fun validProfile(): JSONObject = JSONObject(
        """
        {
          "version": 1,
          "server_addr": "example.test:3443",
          "username": "alice",
          "client_id": "android-test",
          "saved_credential": {
            "type": "server_sealed",
            "key_id": "login-key",
            "expires_at": "2099-01-01T00:00:00Z",
            "ciphertext": "encrypted"
          },
          "tls": {"min_version": "1.3", "insecure_skip_verify": false},
          "connection": {"dial_timeout_sec": 5},
          "forwards": [{
            "name": "rdp",
            "direction": "client_to_server",
            "listen_addr": "127.0.0.1:13389",
            "server_target": "127.0.0.1:3389"
          }]
        }
        """.trimIndent()
    )

    private fun profileZip(profile: JSONObject): ByteArray {
        val output = ByteArrayOutputStream()
        ZipOutputStream(output).use { zip ->
            zip.putNextEntry(ZipEntry("profile.json"))
            zip.write(profile.toString().toByteArray(Charsets.UTF_8))
            zip.closeEntry()
            zip.putNextEntry(ZipEntry("server.crt"))
            zip.write("not-a-certificate".toByteArray(Charsets.UTF_8))
            zip.closeEntry()
        }
        return output.toByteArray()
    }
}
