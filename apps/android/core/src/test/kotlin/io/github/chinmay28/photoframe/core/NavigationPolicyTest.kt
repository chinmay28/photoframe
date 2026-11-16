package io.github.chinmay28.photoframe.core

import org.junit.Assert.assertEquals
import org.junit.Test

class NavigationPolicyTest {
    private val app = Origin.parse("photoframe.local").getOrThrow()

    private fun check(url: String, want: Navigation) = assertEquals(url, want, NavigationPolicy.decide(app, url))

    @Test
    fun theFramesOwnPagesStayInTheApp() {
        check("http://photoframe.local/", Navigation.IN_APP)
        check("http://photoframe.local/#/sources", Navigation.IN_APP)
        check("http://photoframe.local/api/preview.png", Navigation.IN_APP)
        check("blob:http://photoframe.local/0b1c", Navigation.IN_APP)
        check("data:image/png;base64,AAAA", Navigation.IN_APP)
        check("about:blank", Navigation.IN_APP)
    }

    @Test
    fun otherSitesGoToThePhone() {
        check("https://immich.example.com/albums/1", Navigation.EXTERNAL)
        check("http://photoframe.local:8080/", Navigation.EXTERNAL)
        check("https://photoframe.local/", Navigation.EXTERNAL)
        check("https://github.com/chinmay28", Navigation.EXTERNAL)
        check("mailto:someone@example.com", Navigation.EXTERNAL)
        check("tel:+15555550100", Navigation.EXTERNAL)
    }

    @Test
    fun intentUrlsAreTheirOwnCase() {
        check("intent://photoframe.local/#Intent;scheme=http;package=com.android.chrome;end", Navigation.INTENT)
    }

    @Test
    fun dangerousSchemesAreBlocked() {
        check("javascript:alert(1)", Navigation.BLOCK)
        check("file:///data/data/io.github.chinmay28.photoframe/shared_prefs/photoframe.xml", Navigation.BLOCK)
        check("content://com.android.contacts/contacts", Navigation.BLOCK)
        check("about:config", Navigation.BLOCK)
        check("weird", Navigation.BLOCK)
        check("", Navigation.BLOCK)
    }
}
