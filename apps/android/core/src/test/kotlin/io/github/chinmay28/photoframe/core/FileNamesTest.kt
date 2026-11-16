package io.github.chinmay28.photoframe.core

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class FileNamesTest {
    @Test
    fun sanitize() {
        assertEquals("beach.jpg", FileNames.sanitize("beach.jpg"))
        assertEquals("passwd", FileNames.sanitize("../../etc/passwd"))
        assertEquals("evil.jpg", FileNames.sanitize("C:\\Users\\evil.jpg"))
        assertEquals("hidden.png", FileNames.sanitize(".hidden.png"))
        assertEquals("ab.jpg", FileNames.sanitize("a\u0000b\n.jpg"))
        assertEquals("what.jpg", FileNames.sanitize("what?.jpg"))
        assertEquals("download", FileNames.sanitize("  "))
        assertEquals("download", FileNames.sanitize(null))
        assertEquals("photo.jpg", FileNames.sanitize("", "photo.jpg"))
    }

    @Test
    fun longNamesKeepTheirExtension() {
        val s = FileNames.sanitize("é".repeat(200) + ".jpeg")
        assertTrue(s.endsWith(".jpeg"))
        assertTrue(s.toByteArray().size <= 180)
    }
}
