package com.resultv.android.ui.screens

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.ArrowBack
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import com.resultv.android.R
import com.resultv.android.theme.Brand

private data class FaqSection(val titleRes: Int, val bodyRes: Int)

private val FaqSections = listOf(
    FaqSection(R.string.faq_section_what_title, R.string.faq_section_what_body),
    FaqSection(R.string.faq_section_youtube_title, R.string.faq_section_youtube_body),
    FaqSection(R.string.faq_section_install_title, R.string.faq_section_install_body),
    FaqSection(R.string.faq_section_broke_title, R.string.faq_section_broke_body),
    FaqSection(R.string.faq_section_autodisabled_title, R.string.faq_section_autodisabled_body),
    FaqSection(R.string.faq_section_safety_title, R.string.faq_section_safety_body),
)

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun FaqScreen(onBack: () -> Unit) {
    Scaffold(
        containerColor = Brand.Bg,
        topBar = {
            TopAppBar(
                title = { Text(stringResource(R.string.faq_title), fontWeight = FontWeight.Bold) },
                navigationIcon = {
                    IconButton(onClick = onBack) {
                        Icon(
                            Icons.AutoMirrored.Outlined.ArrowBack,
                            contentDescription = stringResource(R.string.action_back),
                        )
                    }
                },
                colors = TopAppBarDefaults.topAppBarColors(containerColor = Brand.Bg),
            )
        },
    ) { padding ->
        Column(
            modifier = Modifier
                .fillMaxSize()
                .padding(padding)
                .verticalScroll(rememberScrollState())
                .padding(horizontal = 16.dp, vertical = 12.dp),
            verticalArrangement = Arrangement.spacedBy(20.dp),
        ) {
            FaqSections.forEach { section ->
                Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
                    Text(
                        stringResource(section.titleRes),
                        style = MaterialTheme.typography.titleMedium,
                        fontWeight = FontWeight.Bold,
                    )
                    Text(
                        stringResource(section.bodyRes),
                        style = MaterialTheme.typography.bodyMedium,
                        color = Brand.SecondaryText,
                    )
                }
            }
        }
    }
}
