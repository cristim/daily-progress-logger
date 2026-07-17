package com.cristim.dailyprogress

import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import com.cristim.dailyprogress.ui.nav.RootScaffold
import com.cristim.dailyprogress.ui.theme.DailyProgressTheme

class MainActivity : ComponentActivity() {

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        enableEdgeToEdge()
        val app = application as App
        setContent {
            DailyProgressTheme {
                RootScaffold(
                    repository = app.container.coreRepository,
                    dataVersion = app.container.dataVersion,
                )
            }
        }
    }
}
