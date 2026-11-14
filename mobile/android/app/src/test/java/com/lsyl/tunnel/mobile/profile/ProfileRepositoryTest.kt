package com.lsyl.tunnel.mobile.profile

import org.json.JSONObject
import org.junit.Assert.assertArrayEquals
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Assert.fail
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder
import java.io.File

class ProfileRepositoryTest {
    @get:Rule val temporaryFolder = TemporaryFolder()

    @Test
    fun legacyProfileRemainsAvailableWhenNewImportBecomesActive() {
        val root = temporaryFolder.newFolder()
        val legacy = imported("old.example:3443", "alice", byteArrayOf(1, 2, 3))
        ProfileFiles(File(root, "active-profile")).save(legacy)
        val repository = ProfileRepository(root)
        assertEquals("old.example:3443", repository.load()?.profile?.serverAddr)

        repository.save(imported("new.example:3443", "bob", byteArrayOf(4, 5, 6)), "家里")
        val profiles = repository.listProfiles()
        assertEquals(2, profiles.size)
        assertEquals("家里", profiles.first().alias)
        assertTrue(profiles.first().active)
        assertEquals("new.example:3443", repository.load()?.profile?.serverAddr)
        assertArrayEquals(legacy.serverCertBytes, File(root, "active-profile/server.crt").readBytes())

        repository.selectProfile("legacy")
        assertEquals("old.example:3443", repository.load()?.profile?.serverAddr)
        assertTrue(repository.listProfiles().first().active)
    }

    @Test
    fun sameAddressProfilesKeepSeparateCredentialsAndForwards() {
        val repository = ProfileRepository(temporaryFolder.newFolder())
        repository.save(imported("shared.example:3443", "alice", byteArrayOf(1)), "账号 A")
        val first = repository.listProfiles().single().id
        repository.save(imported("shared.example:3443", "bob", byteArrayOf(2)), "账号 B")
        val second = repository.listProfiles().first().id
        assertEquals("bob", repository.load()?.profile?.username)

        repository.updateForwards(listOf(ForwardConfig("new", DIRECTION_CLIENT_TO_SERVER,
            "127.0.0.1:13389", "192.168.1.7:3389")))
        repository.selectProfile(first)
        assertEquals("alice", repository.load()?.profile?.username)
        assertEquals("127.0.0.1:3306", repository.load()?.profile?.forwards?.single()?.serverTarget)
        assertArrayEquals(byteArrayOf(1), repository.load()?.serverCertBytes)

        repository.deleteProfile(first)
        assertEquals(second, repository.listProfiles().single().id)
        assertTrue(repository.listProfiles().single().active)
        assertEquals("bob", repository.load()?.profile?.username)
        repository.deleteProfile(second)
        assertTrue(repository.listProfiles().isEmpty())
        assertNull(repository.load())
    }

    @Test
    fun invalidAliasAndProfileIdDoNotReplaceActiveProfile() {
        val repository = ProfileRepository(temporaryFolder.newFolder())
        repository.save(imported("example.test:3443", "alice", byteArrayOf(1)), "")
        val active = repository.listProfiles().single().id
        try {
            repository.save(imported("other.test:3443", "bob", byteArrayOf(2)), "x".repeat(41))
            fail("expected alias validation")
        } catch (_: ProfileImportException) {
        }
        assertEquals(1, repository.listProfiles().size)
        assertEquals(active, repository.listProfiles().single().id)
        assertFalse(repository.listProfiles().single().alias.isBlank())
        try {
            repository.selectProfile("../outside")
            fail("expected invalid profile id rejection")
        } catch (_: ProfileImportException) {
        }
    }

    private fun imported(address: String, username: String, cert: ByteArray): ImportedProfile {
        val json = JSONObject(
            """{
              "version": 1,
              "server_addr": "$address",
              "username": "$username",
              "client_id": "android-test",
              "saved_credential": {
                "type": "server_sealed", "key_id": "login-key",
                "expires_at": "2099-01-01T00:00:00Z", "ciphertext": "sealed-value"
              },
              "tls": {"ca_cert_file": "server.crt", "server_name": "", "min_version": "1.3"},
              "connection": {"dial_timeout_sec": 5},
              "forwards": [{
                "name": "old", "direction": "client_to_server",
                "listen_addr": "127.0.0.1:13306", "server_target": "127.0.0.1:3306"
              }]
            }""".trimIndent()
        )
        return ImportedProfile(MobileProfile.fromJson(json), json, cert, "test-pin")
    }
}
