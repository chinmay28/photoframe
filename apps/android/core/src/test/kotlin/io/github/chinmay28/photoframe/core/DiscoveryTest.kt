package io.github.chinmay28.photoframe.core

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File

class DiscoveryTest {
    @Test
    fun aResolvedServiceOpensAtItsAddressAndIsSavedByName() {
        val f = Discovery.found("photoframe", "/192.168.1.20", 80)!!
        assertEquals("http://192.168.1.20", f.origin.toString())
        assertEquals("http://photoframe.local", f.address)

        val kitchen = Discovery.found("Kitchen", "192.168.1.21", 8080)!!
        assertEquals("http://kitchen.local:8080", kitchen.address)
    }

    @Test
    fun aServiceNameThatIsNotAHostnameIsSavedByAddress() {
        // Avahi renames a clashing service "photoframe #2".
        val f = Discovery.found("photoframe #2", "192.168.1.22", 80)!!
        assertEquals("http://192.168.1.22", f.address)
    }

    @Test
    fun ipv6LosesItsScopeAndLinkLocalIsUseless() {
        assertEquals("http://[fd00::20]", Discovery.found("photoframe", "fd00::20%wlan0", 80)!!.origin.toString())
        assertNull(Discovery.found("photoframe", "fe80::1%wlan0", 80))
    }

    @Test
    fun unusableAnswersAreDropped() {
        assertNull(Discovery.found("photoframe", null, 80))
        assertNull(Discovery.found("photoframe", "", 80))
        assertNull(Discovery.found("photoframe", "192.168.1.20", 0))
        assertEquals("192.168.1.20", Discovery.found("", "192.168.1.20", 80)!!.name)
    }

    @Test
    fun oneFramePerNamePreferringIpv4() {
        val merged = Discovery.merge(
            listOf(
                Discovery.found("photoframe", "fd00::20", 80)!!,
                Discovery.found("Attic", "192.168.1.30", 80)!!,
                Discovery.found("photoframe", "192.168.1.20", 80)!!,
            ),
        )
        assertEquals(listOf("Attic", "photoframe"), merged.map { it.name })
        assertEquals("http://192.168.1.20", merged[1].origin.toString())
    }

    @Test
    fun localNamesMatchTheServiceNamedAfterTheHost() {
        assertTrue(Discovery.isLocalName("photoframe.local"))
        assertTrue(Discovery.isLocalName("PhotoFrame.LOCAL."))
        assertFalse(Discovery.isLocalName("192.168.1.20"))
        assertFalse(Discovery.isLocalName("frame.example.com"))
        assertTrue(Discovery.isServiceFor("photoframe", "photoframe.local"))
        assertTrue(Discovery.isServiceFor("PhotoFrame", "photoframe.local"))
        assertFalse(Discovery.isServiceFor("photoframe #2", "photoframe.local"))
        assertFalse(Discovery.isServiceFor("photoframe", "photoframe.example.com"))
        assertEquals("photoframe.local", Discovery.hostFor("Photoframe"))
        assertNull(Discovery.hostFor("-bad"))
    }

    // The frame side of discovery is the Avahi service file deploy/install.sh
    // writes; the type there and the one the app asks for must be the same.
    @Test
    fun theInstallerAdvertisesTheTypeTheAppLooksFor() {
        val install = File("../../../deploy/install.sh").readText()
        val type = Discovery.SERVICE_TYPE.removeSuffix(".")
        assertTrue("deploy/install.sh does not advertise $type", install.contains("<type>$type</type>"))
        assertTrue("the service is not named after the host", install.contains("<name replace-wildcards=\"yes\">%h</name>"))
    }
}
