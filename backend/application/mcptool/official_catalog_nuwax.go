/*
 * Copyright 2025 coze-dev Authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package mcptool

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	toolapi "github.com/coze-dev/coze-studio/backend/api/model/workbench/tool"
)

const (
	officialMCPSourceCoze              = "coze-official"
	officialMCPSourceNuwaxEcosystem    = "nuwax-ecosystem"
	officialMCPCatalogIDNuwaxFetch     = "nuwax-619e9c7f66c1"
	officialMCPCatalogIDNuwaxNewsNow   = "nuwax-d7b97d686a31"
	officialMCPCatalogIDNuwax12306     = "nuwax-bb2f262dd66b"
	officialMCPCatalogIDNuwaxTime      = "nuwax-33b93476cf59"
	officialMCPCatalogIDNuwaxScholarly = "nuwax-affeb4c7a7ef"
)

func officialMCPCatalogDefinitions() []*officialMCPCatalogDefinition {
	definitions := append(baseOfficialMCPCatalogDefinitions(), nuwaxEcosystemOfficialCatalogDefinitions()...)
	for _, definition := range definitions {
		if definition.Source == "" {
			definition.Source = officialMCPSourceCoze
		}
		if definition.Publisher == "" {
			definition.Publisher = "Coze 官方"
		}
		if definition.Availability == "" {
			definition.Availability = toolapi.MCPOfficialAvailabilityInstallable
		}
	}
	return definitions
}

func nuwaxEcosystemOfficialCatalogDefinitions() []*officialMCPCatalogDefinition {
	return []*officialMCPCatalogDefinition{
		{
			CatalogID:          "nuwax-6e936c006e39",
			Name:               "音视频内容提取服务",
			PersistedName:      "nuwax-official-6e936c006e39",
			Description:        "给定音视频的网络URL地址，提取其中的文本内容，支持异步和同步提取两种方式",
			IconURL:            "https://agent-1251073634.cos.ap-chengdu.myqcloud.com/store/49ddfe68ce08434a8d15a9e59be05501.png",
			Publisher:          "女娲官方",
			Source:             officialMCPSourceNuwaxEcosystem,
			ServerType:         "stdio",
			Availability:       toolapi.MCPOfficialAvailabilityAdapterRequired,
			AvailabilityReason: "Nuwax 生态目录未公开可独立部署的运行模板，需完成 Provider 适配后启用。",
		},
		{
			CatalogID:          "nuwax-cc8fadd0f55c",
			Name:               "图像服务",
			PersistedName:      "nuwax-official-cc8fadd0f55c",
			Description:        "图像MCP服务：包括图像理解、图像生成、图像编辑、OCR",
			IconURL:            "https://agent-1251073634.cos.ap-chengdu.myqcloud.com/store/1f5f378cff874f438b87eb90b40f2d7f.png",
			Publisher:          "女娲官方",
			Source:             officialMCPSourceNuwaxEcosystem,
			ServerType:         "sse",
			Availability:       toolapi.MCPOfficialAvailabilityAdapterRequired,
			AvailabilityReason: "该服务依赖 Nuwax 私有运行网关，需先配置独立 Provider 适配后才能启用。",
		},
		{
			CatalogID:          "nuwax-e9c0831d2591",
			Name:               "PDF内容提取",
			PersistedName:      "nuwax-official-e9c0831d2591",
			Description:        "PDF内容提取，支持含图片的复杂PDF解析提取",
			IconURL:            "https://agent-1251073634.cos.ap-chengdu.myqcloud.com/store/581c74bdf6b34258a4d547eb721124f9.png",
			Publisher:          "女娲官方",
			Source:             officialMCPSourceNuwaxEcosystem,
			ServerType:         "sse",
			Availability:       toolapi.MCPOfficialAvailabilityAdapterRequired,
			AvailabilityReason: "该服务依赖 Nuwax 私有运行网关，需先配置独立 Provider 适配后才能启用。",
		},
		{
			CatalogID:          "nuwax-f30b4fa151f2",
			Name:               "联网搜索",
			PersistedName:      "nuwax-official-f30b4fa151f2",
			Description:        "搜索互联网内容",
			IconURL:            "https://agent-1251073634.cos.ap-chengdu.myqcloud.com/store/0e7bf2641fe3446f9cb85a2846cf6541.png",
			Publisher:          "女娲官方",
			Source:             officialMCPSourceNuwaxEcosystem,
			ServerType:         "sse",
			Availability:       toolapi.MCPOfficialAvailabilityAdapterRequired,
			AvailabilityReason: "该服务依赖 Nuwax 私有运行网关，需先配置独立 Provider 适配后才能启用。",
		},
		{
			CatalogID:     "nuwax-bb2f262dd66b",
			Name:          "12306 车票查询",
			PersistedName: "nuwax-official-bb2f262dd66b",
			Description:   "通过此服务允许用户搜索 12306 的车票信息",
			IconURL:       "https://agent-1251073634.cos.ap-chengdu.myqcloud.com/store/a17f6c72f32a4253aca306aadcd69959.png",
			Publisher:     "女娲官方",
			Source:        officialMCPSourceNuwaxEcosystem,
			ServerType:    "stdio",
			Availability:  toolapi.MCPOfficialAvailabilityInstallable,
		},
		{
			CatalogID:          "nuwax-b2713a01c40b",
			Name:               "企业信息查询",
			PersistedName:      "nuwax-official-b2713a01c40b",
			Description:        "免费查询全国各行业企业基本信息，仅支持查询20年之前的数据。",
			IconURL:            "https://agent-1251073634.cos.ap-chengdu.myqcloud.com/store/79fc9746c8774494a791cd557984a7b2.png",
			Publisher:          "女娲官方",
			Source:             officialMCPSourceNuwaxEcosystem,
			ServerType:         "stdio",
			Availability:       toolapi.MCPOfficialAvailabilityAdapterRequired,
			AvailabilityReason: "Nuwax 生态目录未公开可独立部署的运行模板，需完成 Provider 适配后启用。",
		},
		{
			CatalogID:          "nuwax-292b53b569f6",
			Name:               "数据导出服务",
			PersistedName:      "nuwax-official-292b53b569f6",
			Description:        "支持将数据导出为HTML、Excel、Word（WPS）",
			IconURL:            "https://agent-1251073634.cos.ap-chengdu.myqcloud.com/store/8b6a464d8e214880ab94eb4faba0b44d.png",
			Publisher:          "女娲官方",
			Source:             officialMCPSourceNuwaxEcosystem,
			ServerType:         "sse",
			Availability:       toolapi.MCPOfficialAvailabilityAdapterRequired,
			AvailabilityReason: "该服务依赖 Nuwax 私有运行网关，需先配置独立 Provider 适配后才能启用。",
		},
		{
			CatalogID:          "nuwax-ece1aa09005c",
			Name:               "JSON对比",
			PersistedName:      "nuwax-official-ece1aa09005c",
			Description:        "基于 Model Context Protocol (MCP) 的json对比小工具",
			IconURL:            "https://agent-1251073634.cos.ap-chengdu.myqcloud.com/store/1df4587b525f4698bb63d7c86c90c63d.png",
			Publisher:          "女娲官方",
			Source:             officialMCPSourceNuwaxEcosystem,
			ServerType:         "stdio",
			Availability:       toolapi.MCPOfficialAvailabilityAdapterRequired,
			AvailabilityReason: "Nuwax 生态目录未公开可独立部署的运行模板，需完成 Provider 适配后启用。",
		},
		{
			CatalogID:          "nuwax-ceff4c73b708",
			Name:               "WHOIS",
			PersistedName:      "nuwax-official-ceff4c73b708",
			Description:        "WHOIS 查询是查询 WHOIS 数据库以获取关于域名、IP 地址或自治系统的注册详情的过程。它帮助用户了解谁拥有一个域名、何时注册、何时到期以及其他重要信息。",
			IconURL:            "https://agent-1251073634.cos.ap-chengdu.myqcloud.com/store/35e2aad4cc5449baa76bd1f7361f1db9.png",
			Publisher:          "女娲官方",
			Source:             officialMCPSourceNuwaxEcosystem,
			ServerType:         "stdio",
			Availability:       toolapi.MCPOfficialAvailabilityAdapterRequired,
			AvailabilityReason: "Nuwax 生态目录未公开可独立部署的运行模板，需完成 Provider 适配后启用。",
		},
		{
			CatalogID:          "nuwax-16ffb97030d8",
			Name:               "抖音小助手",
			PersistedName:      "nuwax-official-16ffb97030d8",
			Description:        "一个可以从抖音分享链接下载无水印视频，提取音频并转换为文本的MCP服务。",
			IconURL:            "https://agent-1251073634.cos.ap-chengdu.myqcloud.com/store/a2b54ce9d4c94b9d96419515d26536ca.png",
			Publisher:          "女娲官方",
			Source:             officialMCPSourceNuwaxEcosystem,
			ServerType:         "stdio",
			Availability:       toolapi.MCPOfficialAvailabilityAdapterRequired,
			AvailabilityReason: "Nuwax 生态目录未公开可独立部署的运行模板，需完成 Provider 适配后启用。",
		},
		{
			CatalogID:          "nuwax-b980404a4307",
			Name:               "Bilibili视频信息",
			PersistedName:      "nuwax-official-b980404a4307",
			Description:        "一个可以获取 Bilibili 视频的字幕、弹幕和评论信息的MCP服务。",
			IconURL:            "https://agent-1251073634.cos.ap-chengdu.myqcloud.com/store/441bb653366f4c6c83f1be19ebebf387.png",
			Publisher:          "女娲官方",
			Source:             officialMCPSourceNuwaxEcosystem,
			ServerType:         "stdio",
			Availability:       toolapi.MCPOfficialAvailabilityAdapterRequired,
			AvailabilityReason: "Nuwax 生态目录未公开可独立部署的运行模板，需完成 Provider 适配后启用。",
		},
		{
			CatalogID:          "nuwax-b9a56b442449",
			Name:               "100快递",
			PersistedName:      "nuwax-official-b9a56b442449",
			Description:        "快递100 MCP Server 通过简单配置即可快速接入快递查询、运费预估、智能时效预估（含全程与在途模式）等核心功能。其AI Agent不仅显著降低了开发过程中物流数据服务调用的门槛，提高了开发效率，还增强了对各大行业的物流数据赋能，助力其创新与发展。",
			IconURL:            "https://agent-1251073634.cos.ap-chengdu.myqcloud.com/store/ff241992bcac462ea1a679c01700172e.png",
			Publisher:          "女娲官方",
			Source:             officialMCPSourceNuwaxEcosystem,
			ServerType:         "stdio",
			Availability:       toolapi.MCPOfficialAvailabilityAdapterRequired,
			AvailabilityReason: "Nuwax 生态目录未公开可独立部署的运行模板，需完成 Provider 适配后启用。",
		},
		{
			CatalogID:          "nuwax-fe805ad61a21",
			Name:               "科学计算",
			PersistedName:      "nuwax-official-fe805ad61a21",
			Description:        "一个为用户提供便捷、准确的各类数学运算功能，涵盖了基础运算到复杂三角函数等多种计算需求，适合学生、科研人员以及任何需要进行数学计算的场景使用。",
			IconURL:            "https://agent-1251073634.cos.ap-chengdu.myqcloud.com/store/809d80df89024b1ca1ec6b4d0be58d71.png",
			Publisher:          "女娲官方",
			Source:             officialMCPSourceNuwaxEcosystem,
			ServerType:         "stdio",
			Availability:       toolapi.MCPOfficialAvailabilityAdapterRequired,
			AvailabilityReason: "Nuwax 生态目录未公开可独立部署的运行模板，需完成 Provider 适配后启用。",
		},
		{
			CatalogID:          "nuwax-7048619271bf",
			Name:               "发现报告",
			PersistedName:      "nuwax-official-7048619271bf",
			Description:        "提供对发现报告网站的搜索与内容提取能力，适用于金融、产业研究、投资分析等场景。",
			IconURL:            "https://agent-1251073634.cos.ap-chengdu.myqcloud.com/store/eeea2101d0144000b6e717cac44235f4.png",
			Publisher:          "女娲官方",
			Source:             officialMCPSourceNuwaxEcosystem,
			ServerType:         "stdio",
			Availability:       toolapi.MCPOfficialAvailabilityAdapterRequired,
			AvailabilityReason: "Nuwax 生态目录未公开可独立部署的运行模板，需完成 Provider 适配后启用。",
		},
		{
			CatalogID:     "nuwax-d7b97d686a31",
			Name:          "今日热点",
			PersistedName: "nuwax-official-d7b97d686a31",
			Description:   "极速查询各个热门网站媒体的今日热点",
			IconURL:       "https://agent-1251073634.cos.ap-chengdu.myqcloud.com/store/419b3d15258f4b07990cdc1747feecc0.jpg",
			Publisher:     "女娲官方",
			Source:        officialMCPSourceNuwaxEcosystem,
			ServerType:    "stdio",
			Availability:  toolapi.MCPOfficialAvailabilityInstallable,
			CredentialFields: []*toolapi.MCPOfficialCredentialField{
				{
					Key:         "base_url",
					Label:       "NewsNow 服务地址",
					Required:    true,
					Description: "NewsNow MCP Server 访问的数据服务地址。",
					Placeholder: "https://news.example.com",
				},
			},
		},
		{
			CatalogID:     "nuwax-affeb4c7a7ef",
			Name:          "学术文章搜索服务",
			PersistedName: "nuwax-official-affeb4c7a7ef",
			Description:   "专注于学术文章搜索的 MCP 服务器，旨在帮助用户快速找到相关的学术文献。",
			IconURL:       "https://agent-1251073634.cos.ap-chengdu.myqcloud.com/store/80c2930feb084a05963ca355d7c41842.png",
			Publisher:     "女娲官方",
			Source:        officialMCPSourceNuwaxEcosystem,
			ServerType:    "stdio",
			Availability:  toolapi.MCPOfficialAvailabilityInstallable,
		},
		{
			CatalogID:          "nuwax-1ecb3ba10a16",
			Name:               "高德地图",
			PersistedName:      "nuwax-official-1ecb3ba10a16",
			Description:        "用户可以轻松的利用高德地图 MCP 获取各种基于位置的服务调用",
			IconURL:            "https://agent-1251073634.cos.ap-chengdu.myqcloud.com/store/76c46fff847547f59502eec681e14c3a.png",
			Publisher:          "女娲官方",
			Source:             officialMCPSourceNuwaxEcosystem,
			ServerType:         "stdio",
			Availability:       toolapi.MCPOfficialAvailabilityAdapterRequired,
			AvailabilityReason: "Nuwax 生态目录未公开可独立部署的运行模板，需完成 Provider 适配后启用。",
		},
		{
			CatalogID:          "nuwax-0750c665574f",
			Name:               "百度地图",
			PersistedName:      "nuwax-official-0750c665574f",
			Description:        "百度地图提供的MCP服务，涵盖逆地理编码、地点检索、路线规划等功能",
			IconURL:            "https://agent-1251073634.cos.ap-chengdu.myqcloud.com/store/aa3a3f10165547dab959f5aa9e2a8fed.png",
			Publisher:          "女娲官方",
			Source:             officialMCPSourceNuwaxEcosystem,
			ServerType:         "stdio",
			Availability:       toolapi.MCPOfficialAvailabilityAdapterRequired,
			AvailabilityReason: "Nuwax 生态目录未公开可独立部署的运行模板，需完成 Provider 适配后启用。",
		},
		{
			CatalogID:     "nuwax-33b93476cf59",
			Name:          "Time",
			PersistedName: "nuwax-official-33b93476cf59",
			Description:   "一个提供时间和时区转换功能的模型上下文协议服务器。该服务器使LLM能够获取当前时间信息，并使用IANA时区名称执行时区转换，同时支持自动系统时区检测。",
			IconURL:       "https://agent-1251073634.cos.ap-chengdu.myqcloud.com/store/f3dc67fbc3f04a879ca29ce3ae6d3b52.png",
			Publisher:     "女娲官方",
			Source:        officialMCPSourceNuwaxEcosystem,
			ServerType:    "stdio",
			Availability:  toolapi.MCPOfficialAvailabilityInstallable,
		},
		{
			CatalogID:          "nuwax-2ff50e1f5f22",
			Name:               "MathMind视频剪辑",
			PersistedName:      "nuwax-official-2ff50e1f5f22",
			Description:        "视频合成、剪辑、素材创作等一系列工具。支持图生视频、一张或多张图片合成视频、一个或多个视频合成视频、支持将配音、背景音乐等进行合成，支持字幕识别、字幕打轴等功能。",
			IconURL:            "https://agent-1251073634.cos.ap-chengdu.myqcloud.com/store/ab2142e7a9e64403a350309e02c2cadc.png",
			Publisher:          "女娲官方",
			Source:             officialMCPSourceNuwaxEcosystem,
			ServerType:         "stdio",
			Availability:       toolapi.MCPOfficialAvailabilityAdapterRequired,
			AvailabilityReason: "Nuwax 生态目录未公开可独立部署的运行模板，需完成 Provider 适配后启用。",
		},
		{
			CatalogID:          "nuwax-d8bb4ec842ea",
			Name:               "A股股票查询",
			PersistedName:      "nuwax-official-d8bb4ec842ea",
			Description:        "基于 MCP 协议的A股量化分析工具，为 AI Agent 提供股票推荐、行情分析、K线图绘制等功能",
			IconURL:            "https://agent-1251073634.cos.ap-chengdu.myqcloud.com/store/8705471805f3436785d21c67fa7e1b04.jpg",
			Publisher:          "女娲官方",
			Source:             officialMCPSourceNuwaxEcosystem,
			ServerType:         "stdio",
			Availability:       toolapi.MCPOfficialAvailabilityAdapterRequired,
			AvailabilityReason: "Nuwax 生态目录未公开可独立部署的运行模板，需完成 Provider 适配后启用。",
		},
		{
			CatalogID:     "nuwax-619e9c7f66c1",
			Name:          "Fetch 网页内容抓取",
			PersistedName: "nuwax-official-619e9c7f66c1",
			Description:   "检索和处理网页内容，将HTML转换为markdown格式输出",
			IconURL:       "https://agent-1251073634.cos.ap-chengdu.myqcloud.com/store/b504fb03755347c88f356d31c939951b.png",
			Publisher:     "女娲官方",
			Source:        officialMCPSourceNuwaxEcosystem,
			ServerType:    "stdio",
			Availability:  toolapi.MCPOfficialAvailabilityInstallable,
		},
		{
			CatalogID:          "nuwax-8e5cb10e624a",
			Name:               "Tavily AI 互联网搜索",
			PersistedName:      "nuwax-official-8e5cb10e624a",
			Description:        "增强 AI (LLM) 助手的能力，使其具备实时网络搜索和智能数据提取功能",
			IconURL:            "https://agent-1251073634.cos.ap-chengdu.myqcloud.com/store/eb75946992c94352b13fbdb5753ab4f2.png",
			Publisher:          "女娲官方",
			Source:             officialMCPSourceNuwaxEcosystem,
			ServerType:         "stdio",
			Availability:       toolapi.MCPOfficialAvailabilityAdapterRequired,
			AvailabilityReason: "Nuwax 生态目录未公开可独立部署的运行模板，需完成 Provider 适配后启用。",
		},
		{
			CatalogID:          "nuwax-2612de63e920",
			Name:               "智谱互联网搜索",
			PersistedName:      "nuwax-official-2612de63e920",
			Description:        "实时连接智谱搜索、搜狗搜索、夸克搜索、必应搜索和 Jina AI 搜索，并添有意图识别能力",
			IconURL:            "https://agent-1251073634.cos.ap-chengdu.myqcloud.com/store/02c0c0c0c11e41ffb7e2b08226fa2cc9.png",
			Publisher:          "女娲官方",
			Source:             officialMCPSourceNuwaxEcosystem,
			ServerType:         "stdio",
			Availability:       toolapi.MCPOfficialAvailabilityAdapterRequired,
			AvailabilityReason: "Nuwax 生态目录未公开可独立部署的运行模板，需完成 Provider 适配后启用。",
		},
	}
}

func buildNuwaxOfficialMCPConfig(definition *officialMCPCatalogDefinition, credentials map[string]string) (string, bool, error) {
	if definition.Source != officialMCPSourceNuwaxEcosystem {
		return "", false, nil
	}
	if definition.Availability != toolapi.MCPOfficialAvailabilityInstallable {
		return "", true, fmt.Errorf("official MCP %q requires a platform adapter", definition.Name)
	}

	command := ""
	args := []string{}
	env := map[string]string{}
	switch definition.CatalogID {
	case officialMCPCatalogIDNuwaxFetch:
		command = "uvx"
		args = []string{"mcp-server-fetch"}
	case officialMCPCatalogIDNuwaxNewsNow:
		baseURL, err := validateOfficialMCPHTTPURL(credentials["base_url"])
		if err != nil {
			return "", true, err
		}
		command = "npx"
		args = []string{"-y", "newsnow-mcp-server"}
		env["BASE_URL"] = baseURL
	case officialMCPCatalogIDNuwax12306:
		command = "npx"
		args = []string{"-y", "12306-mcp"}
	case officialMCPCatalogIDNuwaxTime:
		command = "uvx"
		args = []string{"mcp-server-time", "--local-timezone=Asia/Shanghai"}
	case officialMCPCatalogIDNuwaxScholarly:
		command = "uvx"
		args = []string{"mcp-scholarly"}
	default:
		return "", true, fmt.Errorf("official MCP %q has no verified runtime template", definition.Name)
	}

	server := map[string]any{
		"command": command,
		"args":    args,
	}
	if len(env) > 0 {
		server["env"] = env
	}
	payload, err := json.Marshal(server)
	if err != nil {
		return "", true, fmt.Errorf("marshal Nuwax official MCP config: %w", err)
	}
	return string(payload), true, nil
}

func validateOfficialMCPHTTPURL(rawURL string) (string, error) {
	value := strings.TrimSpace(rawURL)
	if value == "" {
		return "", fmt.Errorf("NewsNow service URL is required")
	}
	parsed, err := url.ParseRequestURI(value)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", fmt.Errorf("NewsNow service URL must be a valid HTTP or HTTPS URL")
	}
	return strings.TrimRight(value, "/"), nil
}
