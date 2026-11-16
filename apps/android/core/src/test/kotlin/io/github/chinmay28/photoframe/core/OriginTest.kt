package io.github.chinmay28.photoframe.core

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class OriginTest {

    private fun parsed(input: String) = Origin.parse(input).getOrThrow().toString()

    @Test
    fun aBareHostIsTheFramesPort80OverHttp() {
        assertEquals("http://photoframe.local", parsed("photoframe.local"))
        assertEquals("http://photoframe.local", parsed(Origin.DEFAULT_ADDRESS))
        assertEquals(80, Origin.parse("photoframe.local").getOrThrow().port)
        assertEquals("http://192.168.1.20", parsed(" 192.168.1.20 "))
        assertEquals("http://localhost:8080", parsed("localhost:8080"))
    }

    @Test
    fun aSchemeSpelledOutKeepsItsOwnDefaultPort() {
        assertEquals("http://photoframe.local", parsed("http://photoframe.local"))
        assertEquals("https://frame.example.com", parsed("https://frame.example.com"))
        assertEquals("https://frame.example.com:8443", parsed("HTTPS://Frame.Example.com:8443/"))
    }

    @Test
    fun pathQueryAndFragmentAreDropped() {
        assertEquals("http://photoframe.local", parsed("http://photoframe.local/#/now?x=1"))
    }

    @Test
    fun ipv6IsBracketed() {
        val o = Origin.parse("http://[fd00::20]:8080").getOrThrow()
        assertEquals("fd00::20", o.host)
        assertEquals("http://[fd00::20]:8080", o.toString())
        assertEquals("http://[fd00::20]:8080/api/status", o.url("api/status"))
    }

    @Test
    fun badAddressesAreRefusedWithAReason() {
        for (bad in listOf("", "   ", "ftp://photoframe.local", "http://user:pw@photoframe.local", "http://", "http://photoframe.local:99999")) {
            assertTrue("accepted $bad", Origin.parse(bad).isFailure)
        }
        assertTrue(Origin.parse("").exceptionOrNull()!!.message!!.isNotBlank())
    }

    @Test
    fun containsIsSameOrigin() {
        val o = Origin.parse("photoframe.local").getOrThrow()
        assertTrue(o.contains("http://photoframe.local/api/photos/abc/thumb"))
        assertTrue(o.contains("http://PHOTOFRAME.local:80/"))
        assertFalse(o.contains("http://photoframe.local:8080/"))
        assertFalse(o.contains("https://photoframe.local/"))
        assertFalse(o.contains("http://photoframe.local.evil.test/"))
        assertFalse(o.contains("http://evil.test/?http://photoframe.local"))
        assertFalse(o.contains("not a url"))
    }

    @Test
    fun ofReadsDefaultPorts() {
        assertEquals(Origin("https", "a.test", 443), Origin.of("https://a.test/x"))
        assertEquals(Origin("http", "a.test", 80), Origin.of("http://a.test"))
        assertNull(Origin.of("blob:http://a.test/123"))
        assertNull(Origin.of("mailto:me@a.test"))
    }

    @Test
    fun withHostKeepsSchemeAndPort() {
        val o = Origin.parse("photoframe.local:8080").getOrThrow()
        assertEquals("http://192.168.1.20:8080", o.withHost("192.168.1.20").toString())
        assertEquals("http://[fd00::20]:8080", o.withHost("[FD00::20]").toString())
    }
}
