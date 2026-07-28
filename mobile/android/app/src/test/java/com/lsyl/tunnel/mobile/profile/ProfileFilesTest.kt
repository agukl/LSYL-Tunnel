package com.lsyl.tunnel.mobile.profile

import org.json.JSONObject
import org.junit.Assert.assertArrayEquals
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Assert.fail
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder

class ProfileFilesTest {
    @get:Rule
    val temporaryFolder = TemporaryFolder()

    @Test
    fun updatingForwardsPreservesProtectedProfileDataAndCertificate() {
        val dir = temporaryFolder.newFolder("active-profile")
        val profileFile = dir.resolve("profile.json")
        val certFile = dir.resolve("server.crt")
        val original = validProfile().put("future_server_field", JSONObject().put("enabled", true))
        val certificate = byteArrayOf(1, 3, 3, 7)
        profileFile.writeText(original.toString(), Charsets.UTF_8)
        certFile.writeBytes(certificate)
        val files = ProfileFiles(dir)

        files.updateForwards(
            listOf(
                ForwardConfig(
                    name = "internal-rdp",
                    direction = DIRECTION_CLIENT_TO_SERVER,
                    listenAddr = "127.0.0.1:13389",
                    serverTarget = "192.168.1.7:3389"
                )
            )
        )

        val updated = JSONObject(profileFile.readText(Charsets.UTF_8))
        assertEquals("example.test:3443", updated.getString("server_addr"))
        assertEquals("sealed-value", updated.getJSONObject("saved_credential").getString("ciphertext"))
        assertTrue(updated.getJSONObject("future_server_field").getBoolean("enabled"))
        assertEquals("192.168.1.7:3389", updated.getJSONArray("forwards").getJSONObject(0).getString("server_target"))
        assertArrayEquals(certificate, certFile.readBytes())
        assertFalse(dir.resolve("profile.json.tmp").exists())
    }

    @Test
    fun invalidForwardReplacementLeavesProfileByteForByteUnchanged() {
        val dir = temporaryFolder.newFolder("active-profile-invalid")
        val profileFile = dir.resolve("profile.json")
        val certFile = dir.resolve("server.crt")
        profileFile.writeText(validProfile().toString(2), Charsets.UTF_8)
        certFile.writeBytes(byteArrayOf(9, 8, 7))
        val profileBefore = profileFile.readBytes()
        val certBefore = certFile.readBytes()

        try {
            ProfileFiles(dir).updateForwards(
                listOf(
                    ForwardConfig(
                        name = "unsafe",
                        direction = DIRECTION_CLIENT_TO_SERVER,
                        listenAddr = "0.0.0.0:13389",
                        serverTarget = "192.168.1.7:3389"
                    )
                )
            )
            fail("expected unsafe listener to be rejected")
        } catch (_: ProfileImportException) {
        }

        assertArrayEquals(profileBefore, profileFile.readBytes())
        assertArrayEquals(certBefore, certFile.readBytes())
        assertFalse(dir.resolve("profile.json.tmp").exists())
    }

    private fun validProfile(): JSONObject = JSONObject(
        """
        {
          "version": 1,
          "profile_id": "test-profile",
          "server_addr": "example.test:3443",
          "username": "alice",
          "client_id": "android-test",
          "saved_credential": {
            "type": "server_sealed",
            "key_id": "login-key",
            "expires_at": "2099-01-01T00:00:00Z",
            "ciphertext": "sealed-value"
          },
          "tls": {
            "ca_cert_file": "server.crt",
            "server_name": "",
            "min_version": "1.3",
            "insecure_skip_verify": false
          },
          "connection": {"dial_timeout_sec": 5},
          "forwards": [{
            "name": "old",
            "direction": "client_to_server",
            "listen_addr": "127.0.0.1:13306",
            "server_target": "127.0.0.1:3306"
          }]
        }
        """.trimIndent()
    )
}
