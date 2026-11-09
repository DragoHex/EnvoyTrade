import json
import os
import re

html_dir = "/Users/msp/MSP/.gemini/antigravity-cli/brain/0912b7f4-3f2e-4b3c-87df-2a091c69768e/scratch/groww_html"
output_file = "/Users/msp/MSP/Projects/EnvoyTrade/gogroww/openapi.json"

def clean_html(text):
    text = re.sub(r"<[^>]+>", " ", text)
    text = text.replace("&quot;", "\"").replace("&#x27;", "'").replace("&amp;", "&").replace("&lt;", "<").replace("&gt;", ">")
    return re.sub(r"\s+", " ", text).strip()

def map_type(raw_type):
    t = raw_type.lower()
    if "int" in t:
        return "integer", "int32"
    if "decimal" in t or "float" in t or "number" in t or "double" in t:
        return "number", "float"
    if "bool" in t:
        return "boolean", None
    if "array" in t or "list" in t or "[]" in t:
        return "array", None
    if "object" in t or "dict" in t:
        return "object", None
    return "string", None

def get_annexures():
    with open(os.path.join(html_dir, "annexures.html")) as f:
        html = f.read()
    
    enums = {}
    parts = re.split(r"<h[23][^>]*>(.*?)</h[23]>", html)
    for i in range(1, len(parts), 2):
        heading = clean_html(parts[i])
        content = parts[i+1]
        rows = re.findall(r"<tr>(.*?)</tr>", content, re.DOTALL)
        items = []
        for r in rows:
            cells = [clean_html(c) for c in re.findall(r"<t[dh][^>]*>(.*?)</t[dh]>", r, re.DOTALL)]
            if len(cells) >= 2 and cells[0].lower() != "value":
                items.append(cells[0])
        if items:
            enums[heading] = items
    return enums

def build_openapi():
    enums = get_annexures()
    
    spec = {
        "openapi": "3.0.3",
        "info": {
            "title": "Groww Trading API",
            "version": "1.0",
            "description": "Comprehensive OpenAPI 3.0.3 specification for the Groww Trading API, extracted directly from official documentation (https://groww.in/trade-api/docs/curl). Covers authentication, daily instrument master CSV, order execution, smart conditional orders (GTT/OCO), portfolio, margin calculation, market quote snapshots, historical candle data, backtesting, and account management.",
            "contact": {
                "name": "Groww Developer Support",
                "url": "https://groww.in/trade-api"
            }
        },
        "servers": [
            {
                "url": "https://api.groww.in",
                "description": "Groww Trading API Gateway"
            },
            {
                "url": "https://growwapi-assets.groww.in",
                "description": "Groww Public Assets & Security Master Gateway"
            }
        ],
        "tags": [
            {"name": "Authentication", "description": "Token generation and session approval flows"},
            {"name": "Instruments", "description": "Daily security master CSV downloads"},
            {"name": "Orders", "description": "Regular and After-Market Order (AMO) execution and status audit"},
            {"name": "Smart Orders", "description": "Good-Til-Triggered (GTT) and One-Cancels-Other (OCO) orders"},
            {"name": "Portfolio", "description": "User demat equity holdings and open positions"},
            {"name": "Margin", "description": "Cash balance limits and basket pre-trade margin calculations"},
            {"name": "Live Data", "description": "Real-time quotes, LTP, OHLC, option chain, and option Greeks"},
            {"name": "Historical Data", "description": "Historical candle bars across intervals"},
            {"name": "Backtesting", "description": "Historical derivative contracts, expiries, and expired candles"},
            {"name": "User", "description": "User profile and exchange segment permissions"}
        ],
        "security": [
            {"BearerAuth": []},
            {"ApiVersion": []}
        ],
        "paths": {},
        "components": {
            "securitySchemes": {
                "BearerAuth": {
                    "type": "http",
                    "scheme": "bearer",
                    "bearerFormat": "JWT",
                    "description": "Access token required for all authenticated endpoints. Injected via 'Authorization: Bearer <token>' header."
                },
                "ApiKeyAuth": {
                    "type": "apiKey",
                    "in": "header",
                    "name": "Authorization",
                    "description": "User API Key used specifically for generating access tokens via /v1/token/api/access."
                },
                "ApiVersion": {
                    "type": "apiKey",
                    "in": "header",
                    "name": "X-API-VERSION",
                    "description": "API Version lock header, e.g. '1.0'."
                }
            },
            "schemas": {}
        }
    }

    # Common Enums from Annexures
    order_status_enum = enums.get("Order Status", ["NEW", "ACKED", "TRIGGER_PENDING", "APPROVED", "REJECTED", "FAILED", "EXECUTED", "DELIVERY_AWAITED", "CANCELLED", "CANCELLATION_REQUESTED", "MODIFICATION_REQUESTED", "COMPLETED"])
    amo_status_enum = enums.get("After Market Order Status", ["NA", "PENDING", "DISPATCHED", "PARKED", "PLACED", "FAILED", "MARKET"])
    exchange_enum = enums.get("Exchange", ["BSE", "NSE"])
    segment_enum = enums.get("Segment", ["CASH", "FNO"])
    order_type_enum = enums.get("Order Type", ["LIMIT", "MARKET", "SL", "SL_M"])
    product_enum = enums.get("Product", ["CNC", "MIS", "NRML"])
    transaction_type_enum = enums.get("Transaction Type", ["BUY", "SELL"])
    validity_enum = enums.get("Validity", ["DAY"])
    interval_enum = enums.get("Candle Interval", ["1minute", "2minute", "3minute", "5minute", "10minute", "15minute", "30minute", "1hour", "4hour", "1day", "1week", "1month"])

    # Register Common Error Response
    spec["components"]["schemas"]["ErrorResponse"] = {
        "type": "object",
        "properties": {
            "status": {"type": "string", "example": "FAILURE"},
            "error_code": {
                "type": "string",
                "enum": ["GA000", "GA001", "GA003", "GA004", "GA005", "GA006", "GA007"],
                "description": "Groww API error code"
            },
            "message": {"type": "string", "description": "Human-readable diagnostic failure message"}
        },
        "required": ["status", "error_code", "message"]
    }

    # 1. AUTH: POST /v1/token/api/access
    spec["components"]["schemas"]["TokenApprovalRequest"] = {
        "type": "object",
        "properties": {
            "key_type": {"type": "string", "enum": ["approval", "totp"], "description": "Authentication mode", "example": "approval"},
            "checksum": {"type": "string", "description": "HMAC-SHA256 signature generated with api_secret and timestamp", "example": "abcdef1234567890"},
            "timestamp": {"type": "string", "description": "Epoch timestamp in seconds (10 digits)", "example": "1719830400"},
            "totp": {"type": "string", "description": "Time-based OTP if key_type is 'totp'", "example": "123456"}
        },
        "required": ["key_type"]
    }
    spec["components"]["schemas"]["TokenResponse"] = {
        "type": "object",
        "properties": {
            "token": {"type": "string", "description": "Generated JWT access token"},
            "tokenRefId": {"type": "string", "description": "Reference ID for the generated token session"},
            "sessionName": {"type": "string", "description": "Configured name of the session"},
            "expiry": {"type": "string", "description": "ISO8601 expiry timestamp (daily 6:00 AM)"},
            "isActive": {"type": "boolean", "description": "Token active status flag"}
        },
        "required": ["token", "tokenRefId", "expiry", "isActive"]
    }
    spec["paths"]["/v1/token/api/access"] = {
        "post": {
            "tags": ["Authentication"],
            "summary": "Generate Access Token",
            "description": "Exchanges an API Key, Secret-derived Checksum and Epoch timestamp (or TOTP) for a daily trading Access Token.",
            "security": [{"ApiKeyAuth": []}],
            "requestBody": {
                "required": True,
                "content": {
                    "application/json": {
                        "schema": {"$ref": "#/components/schemas/TokenApprovalRequest"}
                    }
                }
            },
            "responses": {
                "200": {
                    "description": "Token successfully generated",
                    "content": {
                        "application/json": {
                            "schema": {"$ref": "#/components/schemas/TokenResponse"}
                        }
                    }
                },
                "400": {
                    "description": "Bad Request or invalid checksum/signature",
                    "content": {"application/json": {"schema": {"$ref": "#/components/schemas/ErrorResponse"}}}
                },
                "401": {
                    "description": "Unauthorized API Key",
                    "content": {"application/json": {"schema": {"$ref": "#/components/schemas/ErrorResponse"}}}
                }
            }
        }
    }

    # 2. INSTRUMENTS: GET /instruments/instruments.csv
    spec["components"]["schemas"]["InstrumentRecord"] = {
        "type": "object",
        "description": "Single record inside the daily comma-separated instruments.csv file",
        "properties": {
            "exchange": {"type": "string", "enum": exchange_enum},
            "exchange_token": {"type": "string", "description": "Unique token assigned by exchange"},
            "trading_symbol": {"type": "string", "description": "Trading symbol for placing orders"},
            "groww_symbol": {"type": "string", "description": "Groww internal security symbol"},
            "name": {"type": "string", "description": "Company or instrument name"},
            "instrument_type": {"type": "string", "description": "EQ, IDX, FUT, CE, PE"},
            "segment": {"type": "string", "enum": segment_enum},
            "series": {"type": "string", "description": "Series such as EQ, A, B"},
            "isin": {"type": "string", "description": "International Securities Identification Number"},
            "underlying_symbol": {"type": "string", "description": "Underlying asset symbol for derivatives"},
            "underlying_exchange_token": {"type": "string", "description": "Exchange token of underlying"},
            "lot_size": {"type": "integer", "description": "Lot size for trading"},
            "expiry_date": {"type": "string", "description": "Expiry date for F&O contracts"},
            "strike_price": {"type": "number", "format": "float", "description": "Option strike price"},
            "tick_size": {"type": "number", "format": "float", "description": "Minimum price increment"},
            "freeze_quantity": {"type": "integer", "description": "Exchange quantity freeze limit"},
            "is_reserved": {"type": "boolean"},
            "buy_allowed": {"type": "boolean"},
            "sell_allowed": {"type": "boolean"}
        }
    }
    spec["paths"]["/instruments/instruments.csv"] = {
        "servers": [{"url": "https://growwapi-assets.groww.in"}],
        "get": {
            "tags": ["Instruments"],
            "summary": "Download Security Master CSV",
            "description": "Retrieves the complete, daily updated comma-separated CSV file containing all active tradable instruments across CASH and FNO segments.",
            "security": [],
            "responses": {
                "200": {
                    "description": "Instruments CSV file stream",
                    "content": {
                        "text/csv": {
                            "schema": {
                                "type": "string",
                                "format": "binary"
                            }
                        }
                    }
                }
            }
        }
    }

    # 3. ORDERS:
    # 3.1 POST /v1/order/create
    spec["components"]["schemas"]["OrderCreateRequest"] = {
        "type": "object",
        "properties": {
            "trading_symbol": {"type": "string", "description": "Trading Symbol of the instrument"},
            "quantity": {"type": "integer", "description": "Quantity to buy/sell"},
            "price": {"type": "number", "format": "float", "description": "Limit price (required for LIMIT/SL)"},
            "trigger_price": {"type": "number", "format": "float", "description": "Trigger price (required for SL/SL-M)"},
            "order_type": {"type": "string", "enum": order_type_enum, "description": "MARKET, LIMIT, SL, SL_M"},
            "transaction_type": {"type": "string", "enum": transaction_type_enum, "description": "BUY, SELL"},
            "product": {"type": "string", "enum": product_enum, "description": "CNC, MIS, NRML"},
            "validity": {"type": "string", "enum": validity_enum, "description": "DAY"},
            "exchange": {"type": "string", "enum": exchange_enum, "description": "NSE, BSE"},
            "segment": {"type": "string", "enum": segment_enum, "description": "CASH, FNO"},
            "order_reference_id": {"type": "string", "description": "User provided client reference ID for idempotency"}
        },
        "required": ["trading_symbol", "quantity", "order_type", "transaction_type", "product", "validity", "exchange", "segment"]
    }
    spec["components"]["schemas"]["OrderCreateResponse"] = {
        "type": "object",
        "properties": {
            "status": {"type": "string", "enum": ["SUCCESS", "FAILURE"]},
            "groww_order_id": {"type": "string", "description": "Order ID assigned by Groww"},
            "order_status": {"type": "string", "enum": order_status_enum},
            "order_reference_id": {"type": "string", "description": "Client tracking reference ID"},
            "remark": {"type": "string", "description": "Order placement remarks"}
        },
        "required": ["status", "groww_order_id", "order_status"]
    }
    spec["paths"]["/v1/order/create"] = {
        "post": {
            "tags": ["Orders"],
            "summary": "Place Order",
            "description": "Places a new regular or AMO market/limit order on exchange.",
            "requestBody": {
                "required": True,
                "content": {"application/json": {"schema": {"$ref": "#/components/schemas/OrderCreateRequest"}}}
            },
            "responses": {
                "200": {
                    "description": "Order submitted successfully",
                    "content": {"application/json": {"schema": {"$ref": "#/components/schemas/OrderCreateResponse"}}}
                },
                "400": {"description": "Order validation failed", "content": {"application/json": {"schema": {"$ref": "#/components/schemas/ErrorResponse"}}}}
            }
        }
    }

    # 3.2 POST /v1/order/modify
    spec["components"]["schemas"]["OrderModifyRequest"] = {
        "type": "object",
        "properties": {
            "groww_order_id": {"type": "string", "description": "Groww order ID"},
            "quantity": {"type": "integer", "description": "Updated quantity"},
            "price": {"type": "number", "format": "float", "description": "Updated price"},
            "trigger_price": {"type": "number", "format": "float", "description": "Updated trigger price"},
            "order_type": {"type": "string", "enum": order_type_enum},
            "segment": {"type": "string", "enum": segment_enum}
        },
        "required": ["groww_order_id", "quantity", "order_type", "segment"]
    }
    spec["components"]["schemas"]["OrderActionResponse"] = {
        "type": "object",
        "properties": {
            "status": {"type": "string", "enum": ["SUCCESS", "FAILURE"]},
            "groww_order_id": {"type": "string"},
            "order_status": {"type": "string", "enum": order_status_enum},
            "remark": {"type": "string"}
        },
        "required": ["status", "groww_order_id", "order_status"]
    }
    spec["paths"]["/v1/order/modify"] = {
        "post": {
            "tags": ["Orders"],
            "summary": "Modify Order",
            "description": "Modifies price, trigger price, or quantity of a pending/open order.",
            "requestBody": {
                "required": True,
                "content": {"application/json": {"schema": {"$ref": "#/components/schemas/OrderModifyRequest"}}}
            },
            "responses": {
                "200": {
                    "description": "Order modification submitted",
                    "content": {"application/json": {"schema": {"$ref": "#/components/schemas/OrderActionResponse"}}}
                },
                "400": {"description": "Order not modifiable or invalid parameters", "content": {"application/json": {"schema": {"$ref": "#/components/schemas/ErrorResponse"}}}}
            }
        }
    }

    # 3.3 POST /v1/order/cancel
    spec["components"]["schemas"]["OrderCancelRequest"] = {
        "type": "object",
        "properties": {
            "groww_order_id": {"type": "string", "description": "Groww order ID"},
            "segment": {"type": "string", "enum": segment_enum}
        },
        "required": ["groww_order_id", "segment"]
    }
    spec["paths"]["/v1/order/cancel"] = {
        "post": {
            "tags": ["Orders"],
            "summary": "Cancel Order",
            "description": "Cancels an open or pending order.",
            "requestBody": {
                "required": True,
                "content": {"application/json": {"schema": {"$ref": "#/components/schemas/OrderCancelRequest"}}}
            },
            "responses": {
                "200": {
                    "description": "Order cancellation request accepted",
                    "content": {"application/json": {"schema": {"$ref": "#/components/schemas/OrderActionResponse"}}}
                },
                "400": {"description": "Order cannot be cancelled", "content": {"application/json": {"schema": {"$ref": "#/components/schemas/ErrorResponse"}}}}
            }
        }
    }

    # 3.4 GET /v1/order/status/{groww_order_id}
    # 3.5 GET /v1/order/status/reference/{order_reference_id}
    spec["components"]["schemas"]["OrderStatusDetailResponse"] = {
        "type": "object",
        "properties": {
            "status": {"type": "string", "enum": ["SUCCESS", "FAILURE"]},
            "groww_order_id": {"type": "string"},
            "order_status": {"type": "string", "enum": order_status_enum},
            "order_reference_id": {"type": "string"},
            "remark": {"type": "string"},
            "exchange_order_id": {"type": "string"},
            "rejected_reason": {"type": "string"}
        },
        "required": ["status", "groww_order_id", "order_status"]
    }
    spec["paths"]["/v1/order/status/{groww_order_id}"] = {
        "get": {
            "tags": ["Orders"],
            "summary": "Get Order Status by Order ID",
            "description": "Fetches current execution status for a given Groww Order ID.",
            "parameters": [
                {"name": "groww_order_id", "in": "path", "required": True, "schema": {"type": "string"}},
                {"name": "segment", "in": "query", "required": True, "schema": {"type": "string", "enum": segment_enum}}
            ],
            "responses": {
                "200": {"description": "Order status retrieved", "content": {"application/json": {"schema": {"$ref": "#/components/schemas/OrderStatusDetailResponse"}}}}
            }
        }
    }
    spec["paths"]["/v1/order/status/reference/{order_reference_id}"] = {
        "get": {
            "tags": ["Orders"],
            "summary": "Get Order Status by Reference ID",
            "description": "Fetches current execution status for a given client order reference ID.",
            "parameters": [
                {"name": "order_reference_id", "in": "path", "required": True, "schema": {"type": "string"}},
                {"name": "segment", "in": "query", "required": True, "schema": {"type": "string", "enum": segment_enum}}
            ],
            "responses": {
                "200": {"description": "Order status retrieved", "content": {"application/json": {"schema": {"$ref": "#/components/schemas/OrderStatusDetailResponse"}}}}
            }
        }
    }

    # 3.6 GET /v1/order/detail/{groww_order_id}
    # 3.7 GET /v1/order/list
    spec["components"]["schemas"]["OrderDetail"] = {
        "type": "object",
        "properties": {
            "groww_order_id": {"type": "string"},
            "order_reference_id": {"type": "string"},
            "exchange_order_id": {"type": "string"},
            "trading_symbol": {"type": "string"},
            "groww_symbol": {"type": "string"},
            "exchange": {"type": "string", "enum": exchange_enum},
            "segment": {"type": "string", "enum": segment_enum},
            "product": {"type": "string", "enum": product_enum},
            "order_type": {"type": "string", "enum": order_type_enum},
            "transaction_type": {"type": "string", "enum": transaction_type_enum},
            "validity": {"type": "string", "enum": validity_enum},
            "quantity": {"type": "integer"},
            "filled_quantity": {"type": "integer"},
            "pending_quantity": {"type": "integer"},
            "price": {"type": "number", "format": "float"},
            "trigger_price": {"type": "number", "format": "float"},
            "average_price": {"type": "number", "format": "float"},
            "order_status": {"type": "string", "enum": order_status_enum},
            "amo": {"type": "boolean"},
            "amo_status": {"type": "string", "enum": amo_status_enum},
            "order_placed_time": {"type": "string"},
            "order_modified_time": {"type": "string"},
            "remark": {"type": "string"}
        }
    }
    spec["paths"]["/v1/order/detail/{groww_order_id}"] = {
        "get": {
            "tags": ["Orders"],
            "summary": "Get Order Details",
            "description": "Fetches full metadata and state history of a single order.",
            "parameters": [
                {"name": "groww_order_id", "in": "path", "required": True, "schema": {"type": "string"}},
                {"name": "segment", "in": "query", "required": True, "schema": {"type": "string", "enum": segment_enum}}
            ],
            "responses": {
                "200": {
                    "description": "Order details response",
                    "content": {
                        "application/json": {
                            "schema": {
                                "type": "object",
                                "properties": {
                                    "status": {"type": "string"},
                                    "order": {"$ref": "#/components/schemas/OrderDetail"}
                                }
                            }
                        }
                    }
                }
            }
        }
    }
    spec["paths"]["/v1/order/list"] = {
        "get": {
            "tags": ["Orders"],
            "summary": "Get Order List",
            "description": "Retrieves list of all orders placed during the current trading day.",
            "parameters": [
                {"name": "segment", "in": "query", "required": True, "schema": {"type": "string", "enum": segment_enum}},
                {"name": "page", "in": "query", "required": False, "schema": {"type": "integer", "default": 0}},
                {"name": "page_size", "in": "query", "required": False, "schema": {"type": "integer", "default": 50}}
            ],
            "responses": {
                "200": {
                    "description": "Order book list",
                    "content": {
                        "application/json": {
                            "schema": {
                                "type": "object",
                                "properties": {
                                    "status": {"type": "string"},
                                    "orders": {
                                        "type": "array",
                                        "items": {"$ref": "#/components/schemas/OrderDetail"}
                                    }
                                }
                            }
                        }
                    }
                }
            }
        }
    }

    # 3.8 GET /v1/order/trades/{groww_order_id}
    spec["components"]["schemas"]["TradeRecord"] = {
        "type": "object",
        "properties": {
            "trade_id": {"type": "string"},
            "groww_order_id": {"type": "string"},
            "exchange_order_id": {"type": "string"},
            "trade_quantity": {"type": "integer"},
            "trade_price": {"type": "number", "format": "float"},
            "trade_time": {"type": "string"},
            "trading_symbol": {"type": "string"},
            "exchange": {"type": "string", "enum": exchange_enum},
            "segment": {"type": "string", "enum": segment_enum}
        }
    }
    spec["paths"]["/v1/order/trades/{groww_order_id}"] = {
        "get": {
            "tags": ["Orders"],
            "summary": "Get Order Trades",
            "description": "Retrieves individual execution fill splits for an order.",
            "parameters": [
                {"name": "groww_order_id", "in": "path", "required": True, "schema": {"type": "string"}},
                {"name": "segment", "in": "query", "required": True, "schema": {"type": "string", "enum": segment_enum}}
            ],
            "responses": {
                "200": {
                    "description": "Order trades list",
                    "content": {
                        "application/json": {
                            "schema": {
                                "type": "object",
                                "properties": {
                                    "status": {"type": "string"},
                                    "trades": {"type": "array", "items": {"$ref": "#/components/schemas/TradeRecord"}}
                                }
                            }
                        }
                    }
                }
            }
        }
    }

    # 4. SMART ORDERS (GTT / OCO)
    spec["components"]["schemas"]["SmartOrderCreateRequest"] = {
        "type": "object",
        "properties": {
            "smart_order_type": {"type": "string", "enum": ["GTT", "OCO"], "description": "GTT or OCO"},
            "segment": {"type": "string", "enum": segment_enum},
            "exchange": {"type": "string", "enum": exchange_enum},
            "trading_symbol": {"type": "string"},
            "quantity": {"type": "integer"},
            "product_type": {"type": "string", "enum": product_enum},
            "duration": {"type": "string", "enum": validity_enum},
            "trigger_price": {"type": "string", "description": "Decimal trigger price string for GTT"},
            "trigger_direction": {"type": "string", "enum": ["UP", "DOWN"]},
            "order": {
                "type": "object",
                "properties": {
                    "order_type": {"type": "string", "enum": order_type_enum},
                    "price": {"type": "string"},
                    "transaction_type": {"type": "string", "enum": transaction_type_enum}
                }
            },
            "target": {
                "type": "object",
                "description": "Target leg for OCO",
                "properties": {
                    "trigger_price": {"type": "string"},
                    "order_type": {"type": "string", "enum": order_type_enum},
                    "price": {"type": "string"}
                }
            },
            "stop_loss": {
                "type": "object",
                "description": "Stop Loss leg for OCO",
                "properties": {
                    "trigger_price": {"type": "string"},
                    "order_type": {"type": "string", "enum": order_type_enum},
                    "price": {"type": "string"}
                }
            }
        },
        "required": ["smart_order_type", "segment", "exchange", "trading_symbol", "quantity", "product_type", "duration"]
    }
    spec["components"]["schemas"]["SmartOrderActionResponse"] = {
        "type": "object",
        "properties": {
            "status": {"type": "string"},
            "smart_order_id": {"type": "string"},
            "smart_order_type": {"type": "string", "enum": ["GTT", "OCO"]},
            "message": {"type": "string"}
        }
    }
    spec["paths"]["/v1/order-advance/create"] = {
        "post": {
            "tags": ["Smart Orders"],
            "summary": "Create Smart Order (GTT / OCO)",
            "description": "Creates a Good-Til-Triggered (GTT) or One-Cancels-Other (OCO) conditional order.",
            "requestBody": {
                "required": True,
                "content": {"application/json": {"schema": {"$ref": "#/components/schemas/SmartOrderCreateRequest"}}}
            },
            "responses": {
                "200": {"description": "Smart order created", "content": {"application/json": {"schema": {"$ref": "#/components/schemas/SmartOrderActionResponse"}}}}
            }
        }
    }
    spec["paths"]["/v1/order-advance/modify/{smart_order_id}"] = {
        "put": {
            "tags": ["Smart Orders"],
            "summary": "Modify Smart Order",
            "description": "Modifies parameters of an active smart order.",
            "parameters": [
                {"name": "smart_order_id", "in": "path", "required": True, "schema": {"type": "string"}}
            ],
            "requestBody": {
                "required": True,
                "content": {"application/json": {"schema": {"$ref": "#/components/schemas/SmartOrderCreateRequest"}}}
            },
            "responses": {
                "200": {"description": "Smart order modified", "content": {"application/json": {"schema": {"$ref": "#/components/schemas/SmartOrderActionResponse"}}}}
            }
        }
    }
    spec["paths"]["/v1/order-advance/cancel/{segment}/{smart_order_type}/{smart_order_id}"] = {
        "post": {
            "tags": ["Smart Orders"],
            "summary": "Cancel Smart Order",
            "description": "Cancels an active smart order.",
            "parameters": [
                {"name": "segment", "in": "path", "required": True, "schema": {"type": "string", "enum": segment_enum}},
                {"name": "smart_order_type", "in": "path", "required": True, "schema": {"type": "string", "enum": ["GTT", "OCO"]}},
                {"name": "smart_order_id", "in": "path", "required": True, "schema": {"type": "string"}}
            ],
            "responses": {
                "202": {"description": "Smart order cancellation initiated", "content": {"application/json": {"schema": {"$ref": "#/components/schemas/SmartOrderActionResponse"}}}}
            }
        }
    }
    spec["paths"]["/v1/order-advance/status/{segment}/{smart_order_type}/internal/{smart_order_id}"] = {
        "get": {
            "tags": ["Smart Orders"],
            "summary": "Get Smart Order Status",
            "description": "Fetches current status and triggers of a smart order.",
            "parameters": [
                {"name": "segment", "in": "path", "required": True, "schema": {"type": "string", "enum": segment_enum}},
                {"name": "smart_order_type", "in": "path", "required": True, "schema": {"type": "string", "enum": ["GTT", "OCO"]}},
                {"name": "smart_order_id", "in": "path", "required": True, "schema": {"type": "string"}}
            ],
            "responses": {
                "200": {"description": "Smart order detail", "content": {"application/json": {"schema": {"type": "object"}}}}
            }
        }
    }
    spec["paths"]["/v1/order-advance/list"] = {
        "get": {
            "tags": ["Smart Orders"],
            "summary": "List Smart Orders",
            "description": "Lists all configured smart orders with filtering by segment and status.",
            "parameters": [
                {"name": "segment", "in": "query", "required": False, "schema": {"type": "string", "enum": segment_enum}},
                {"name": "smart_order_type", "in": "query", "required": False, "schema": {"type": "string", "enum": ["GTT", "OCO"]}},
                {"name": "status", "in": "query", "required": False, "schema": {"type": "string", "enum": ["ACTIVE", "CANCELLED", "COMPLETED"]}},
                {"name": "page", "in": "query", "required": False, "schema": {"type": "integer", "default": 0}},
                {"name": "page_size", "in": "query", "required": False, "schema": {"type": "integer", "default": 10}},
                {"name": "start_date_time", "in": "query", "required": False, "schema": {"type": "string"}},
                {"name": "end_date_time", "in": "query", "required": False, "schema": {"type": "string"}}
            ],
            "responses": {
                "200": {"description": "Smart orders list", "content": {"application/json": {"schema": {"type": "object"}}}}
            }
        }
    }

    # 5. PORTFOLIO
    spec["components"]["schemas"]["HoldingRecord"] = {
        "type": "object",
        "properties": {
            "trading_symbol": {"type": "string"},
            "groww_symbol": {"type": "string"},
            "isin": {"type": "string"},
            "quantity": {"type": "integer"},
            "average_price": {"type": "number", "format": "float"},
            "ltp": {"type": "number", "format": "float"},
            "current_value": {"type": "number", "format": "float"},
            "pnl": {"type": "number", "format": "float"},
            "pnl_percentage": {"type": "number", "format": "float"}
        }
    }
    spec["paths"]["/v1/holdings/user"] = {
        "get": {
            "tags": ["Portfolio"],
            "summary": "Get User Holdings",
            "description": "Retrieves all long-term Demat equity holdings.",
            "responses": {
                "200": {
                    "description": "User holdings list",
                    "content": {
                        "application/json": {
                            "schema": {
                                "type": "object",
                                "properties": {
                                    "status": {"type": "string"},
                                    "holdings": {"type": "array", "items": {"$ref": "#/components/schemas/HoldingRecord"}}
                                }
                            }
                        }
                    }
                }
            }
        }
    }

    spec["components"]["schemas"]["PositionRecord"] = {
        "type": "object",
        "properties": {
            "trading_symbol": {"type": "string"},
            "exchange": {"type": "string", "enum": exchange_enum},
            "segment": {"type": "string", "enum": segment_enum},
            "product": {"type": "string", "enum": product_enum},
            "net_quantity": {"type": "integer"},
            "buy_quantity": {"type": "integer"},
            "sell_quantity": {"type": "integer"},
            "buy_price": {"type": "number", "format": "float"},
            "sell_price": {"type": "number", "format": "float"},
            "realised_pnl": {"type": "number", "format": "float"},
            "unrealised_pnl": {"type": "number", "format": "float"}
        }
    }
    spec["paths"]["/v1/positions/user"] = {
        "get": {
            "tags": ["Portfolio"],
            "summary": "Get User Positions",
            "description": "Retrieves day and net positions across CASH and FNO segments.",
            "parameters": [
                {"name": "segment", "in": "query", "required": False, "schema": {"type": "string", "enum": segment_enum}}
            ],
            "responses": {
                "200": {
                    "description": "User positions list",
                    "content": {
                        "application/json": {
                            "schema": {
                                "type": "object",
                                "properties": {
                                    "status": {"type": "string"},
                                    "positions": {"type": "array", "items": {"$ref": "#/components/schemas/PositionRecord"}}
                                }
                            }
                        }
                    }
                }
            }
        }
    }
    spec["paths"]["/v1/positions/trading-symbol"] = {
        "get": {
            "tags": ["Portfolio"],
            "summary": "Get Position by Trading Symbol",
            "description": "Fetches current open position for a specific trading symbol.",
            "parameters": [
                {"name": "trading_symbol", "in": "query", "required": True, "schema": {"type": "string"}},
                {"name": "exchange", "in": "query", "required": True, "schema": {"type": "string", "enum": exchange_enum}},
                {"name": "segment", "in": "query", "required": True, "schema": {"type": "string", "enum": segment_enum}}
            ],
            "responses": {
                "200": {
                    "description": "Position for symbol",
                    "content": {"application/json": {"schema": {"$ref": "#/components/schemas/PositionRecord"}}}
                }
            }
        }
    }

    # 6. MARGIN
    spec["components"]["schemas"]["UserMarginDetail"] = {
        "type": "object",
        "properties": {
            "status": {"type": "string"},
            "available_margin": {"type": "number", "format": "float"},
            "utilized_margin": {"type": "number", "format": "float"},
            "net_margin": {"type": "number", "format": "float"},
            "collateral_margin": {"type": "number", "format": "float"},
            "cash_balance": {"type": "number", "format": "float"}
        }
    }
    spec["paths"]["/v1/margins/detail/user"] = {
        "get": {
            "tags": ["Margin"],
            "summary": "Get Available User Margin",
            "description": "Retrieves cash balances, collateral margin, and current margin utilization.",
            "responses": {
                "200": {"description": "User margin details", "content": {"application/json": {"schema": {"$ref": "#/components/schemas/UserMarginDetail"}}}}
            }
        }
    }
    spec["components"]["schemas"]["MarginOrderCalcItem"] = {
        "type": "object",
        "properties": {
            "trading_symbol": {"type": "string"},
            "quantity": {"type": "integer"},
            "price": {"type": "number", "format": "float"},
            "order_type": {"type": "string", "enum": order_type_enum},
            "transaction_type": {"type": "string", "enum": transaction_type_enum},
            "product": {"type": "string", "enum": product_enum},
            "exchange": {"type": "string", "enum": exchange_enum},
            "segment": {"type": "string", "enum": segment_enum}
        },
        "required": ["trading_symbol", "quantity", "order_type", "transaction_type", "product", "exchange", "segment"]
    }
    spec["components"]["schemas"]["MarginOrderCalcRequest"] = {
        "type": "object",
        "properties": {
            "orders": {
                "type": "array",
                "items": {"$ref": "#/components/schemas/MarginOrderCalcItem"}
            }
        },
        "required": ["orders"]
    }
    spec["paths"]["/v1/margins/detail/orders"] = {
        "post": {
            "tags": ["Margin"],
            "summary": "Calculate Required Margin for Orders",
            "description": "Computes upfront required margin and hedge benefits for a list of prospective orders.",
            "requestBody": {
                "required": True,
                "content": {"application/json": {"schema": {"$ref": "#/components/schemas/MarginOrderCalcRequest"}}}
            },
            "responses": {
                "200": {
                    "description": "Margin requirement computation",
                    "content": {
                        "application/json": {
                            "schema": {
                                "type": "object",
                                "properties": {
                                    "status": {"type": "string"},
                                    "total_margin_required": {"type": "number", "format": "float"},
                                    "span_margin": {"type": "number", "format": "float"},
                                    "exposure_margin": {"type": "number", "format": "float"}
                                }
                            }
                        }
                    }
                }
            }
        }
    }

    # 7. LIVE DATA
    spec["components"]["schemas"]["MarketDepthEntry"] = {
        "type": "object",
        "properties": {
            "price": {"type": "number", "format": "float"},
            "quantity": {"type": "integer"},
            "orders": {"type": "integer"}
        }
    }
    spec["components"]["schemas"]["LiveQuoteResponse"] = {
        "type": "object",
        "properties": {
            "status": {"type": "string"},
            "payload": {
                "type": "object",
                "properties": {
                    "trading_symbol": {"type": "string"},
                    "exchange": {"type": "string", "enum": exchange_enum},
                    "segment": {"type": "string", "enum": segment_enum},
                    "last_price": {"type": "number", "format": "float"},
                    "ohlc": {
                        "type": "object",
                        "properties": {
                            "open": {"type": "number", "format": "float"},
                            "high": {"type": "number", "format": "float"},
                            "low": {"type": "number", "format": "float"},
                            "close": {"type": "number", "format": "float"}
                        }
                    },
                    "depth": {
                        "type": "object",
                        "properties": {
                            "buy": {"type": "array", "items": {"$ref": "#/components/schemas/MarketDepthEntry"}},
                            "sell": {"type": "array", "items": {"$ref": "#/components/schemas/MarketDepthEntry"}}
                        }
                    },
                    "volume": {"type": "integer"},
                    "open_interest": {"type": "integer"},
                    "average_price": {"type": "number", "format": "float"}
                }
            }
        }
    }
    spec["paths"]["/v1/live-data/quote"] = {
        "get": {
            "tags": ["Live Data"],
            "summary": "Get Live Quote Snapshot",
            "description": "Returns full market snapshot including LTP, OHLC, 5-level market depth, and volume.",
            "parameters": [
                {"name": "exchange", "in": "query", "required": True, "schema": {"type": "string", "enum": exchange_enum}},
                {"name": "segment", "in": "query", "required": True, "schema": {"type": "string", "enum": segment_enum}},
                {"name": "trading_symbol", "in": "query", "required": True, "schema": {"type": "string"}}
            ],
            "responses": {
                "200": {"description": "Live quote snapshot", "content": {"application/json": {"schema": {"$ref": "#/components/schemas/LiveQuoteResponse"}}}}
            }
        }
    }
    spec["paths"]["/v1/live-data/ltp"] = {
        "get": {
            "tags": ["Live Data"],
            "summary": "Get Last Traded Price (LTP)",
            "description": "Lightweight endpoint returning latest traded price.",
            "parameters": [
                {"name": "exchange", "in": "query", "required": True, "schema": {"type": "string", "enum": exchange_enum}},
                {"name": "segment", "in": "query", "required": True, "schema": {"type": "string", "enum": segment_enum}},
                {"name": "trading_symbol", "in": "query", "required": True, "schema": {"type": "string"}}
            ],
            "responses": {
                "200": {
                    "description": "LTP response",
                    "content": {
                        "application/json": {
                            "schema": {
                                "type": "object",
                                "properties": {
                                    "status": {"type": "string"},
                                    "payload": {
                                        "type": "object",
                                        "properties": {
                                            "last_price": {"type": "number", "format": "float"}
                                        }
                                    }
                                }
                            }
                        }
                    }
                }
            }
        }
    }
    spec["paths"]["/v1/live-data/ohlc"] = {
        "get": {
            "tags": ["Live Data"],
            "summary": "Get OHLC Snapshot",
            "description": "Returns Open, High, Low, and Close prices for an instrument.",
            "parameters": [
                {"name": "exchange", "in": "query", "required": True, "schema": {"type": "string", "enum": exchange_enum}},
                {"name": "segment", "in": "query", "required": True, "schema": {"type": "string", "enum": segment_enum}},
                {"name": "trading_symbol", "in": "query", "required": True, "schema": {"type": "string"}}
            ],
            "responses": {
                "200": {
                    "description": "OHLC response",
                    "content": {
                        "application/json": {
                            "schema": {
                                "type": "object",
                                "properties": {
                                    "status": {"type": "string"},
                                    "payload": {
                                        "type": "object",
                                        "properties": {
                                            "open": {"type": "number", "format": "float"},
                                            "high": {"type": "number", "format": "float"},
                                            "low": {"type": "number", "format": "float"},
                                            "close": {"type": "number", "format": "float"}
                                        }
                                    }
                                }
                            }
                        }
                    }
                }
            }
        }
    }
    spec["paths"]["/v1/option-chain/exchange/{exchange}/underlying/{underlying}"] = {
        "get": {
            "tags": ["Live Data"],
            "summary": "Get Option Chain",
            "description": "Returns complete option chain matrix for an underlying and expiry date.",
            "parameters": [
                {"name": "exchange", "in": "path", "required": True, "schema": {"type": "string", "enum": exchange_enum}},
                {"name": "underlying", "in": "path", "required": True, "schema": {"type": "string"}},
                {"name": "expiry_date", "in": "query", "required": True, "schema": {"type": "string"}}
            ],
            "responses": {
                "200": {"description": "Option chain matrix", "content": {"application/json": {"schema": {"type": "object"}}}}
            }
        }
    }
    spec["paths"]["/v1/live-data/greeks/exchange/{exchange}/underlying/{underlying}/trading_symbol/{trading_symbol}/expiry/{expiry}"] = {
        "get": {
            "tags": ["Live Data"],
            "summary": "Get Option Greeks",
            "description": "Calculates real-time Greeks (Delta, Gamma, Theta, Vega, IV) for an option contract.",
            "parameters": [
                {"name": "exchange", "in": "path", "required": True, "schema": {"type": "string", "enum": exchange_enum}},
                {"name": "underlying", "in": "path", "required": True, "schema": {"type": "string"}},
                {"name": "trading_symbol", "in": "path", "required": True, "schema": {"type": "string"}},
                {"name": "expiry", "in": "path", "required": True, "schema": {"type": "string"}}
            ],
            "responses": {
                "200": {
                    "description": "Greeks payload",
                    "content": {
                        "application/json": {
                            "schema": {
                                "type": "object",
                                "properties": {
                                    "status": {"type": "string"},
                                    "payload": {
                                        "type": "object",
                                        "properties": {
                                            "delta": {"type": "number", "format": "float"},
                                            "gamma": {"type": "number", "format": "float"},
                                            "theta": {"type": "number", "format": "float"},
                                            "vega": {"type": "number", "format": "float"},
                                            "iv": {"type": "number", "format": "float"}
                                        }
                                    }
                                }
                            }
                        }
                    }
                }
            }
        }
    }

    # 8. HISTORICAL DATA
    spec["components"]["schemas"]["CandleData"] = {
        "type": "object",
        "properties": {
            "time": {"type": "string"},
            "open": {"type": "number", "format": "float"},
            "high": {"type": "number", "format": "float"},
            "low": {"type": "number", "format": "float"},
            "close": {"type": "number", "format": "float"},
            "volume": {"type": "integer"}
        }
    }
    spec["paths"]["/v1/historical/candle/range"] = {
        "get": {
            "tags": ["Historical Data"],
            "summary": "Get Historical Candle Range",
            "description": "Returns historical OHLCV candle bars for a given instrument, interval, and date range.",
            "parameters": [
                {"name": "trading_symbol", "in": "query", "required": True, "schema": {"type": "string"}},
                {"name": "exchange", "in": "query", "required": True, "schema": {"type": "string", "enum": exchange_enum}},
                {"name": "segment", "in": "query", "required": True, "schema": {"type": "string", "enum": segment_enum}},
                {"name": "interval_in_minutes", "in": "query", "required": True, "schema": {"type": "string", "enum": interval_enum}},
                {"name": "start_time", "in": "query", "required": True, "schema": {"type": "string", "description": "ISO8601 or YYYY-MM-DD HH:mm:ss"}},
                {"name": "end_time", "in": "query", "required": True, "schema": {"type": "string", "description": "ISO8601 or YYYY-MM-DD HH:mm:ss"}}
            ],
            "responses": {
                "200": {
                    "description": "Candles array",
                    "content": {
                        "application/json": {
                            "schema": {
                                "type": "object",
                                "properties": {
                                    "status": {"type": "string"},
                                    "candles": {"type": "array", "items": {"$ref": "#/components/schemas/CandleData"}}
                                }
                            }
                        }
                    }
                }
            }
        }
    }

    # 9. BACKTESTING
    spec["paths"]["/v1/historical/expiries"] = {
        "get": {
            "tags": ["Backtesting"],
            "summary": "Get Historical Expiries",
            "description": "Fetches list of expired derivative contract expiry dates for backtesting.",
            "parameters": [
                {"name": "underlying", "in": "query", "required": True, "schema": {"type": "string"}},
                {"name": "exchange", "in": "query", "required": True, "schema": {"type": "string", "enum": exchange_enum}},
                {"name": "segment", "in": "query", "required": True, "schema": {"type": "string", "enum": segment_enum}}
            ],
            "responses": {
                "200": {"description": "Expiries list", "content": {"application/json": {"schema": {"type": "object"}}}}
            }
        }
    }
    spec["paths"]["/v1/historical/contracts"] = {
        "get": {
            "tags": ["Backtesting"],
            "summary": "Get Historical Contracts",
            "description": "Fetches expired derivative contract master specifications for a given underlying and expiry.",
            "parameters": [
                {"name": "underlying", "in": "query", "required": True, "schema": {"type": "string"}},
                {"name": "expiry_date", "in": "query", "required": True, "schema": {"type": "string"}},
                {"name": "exchange", "in": "query", "required": True, "schema": {"type": "string", "enum": exchange_enum}}
            ],
            "responses": {
                "200": {"description": "Contracts list", "content": {"application/json": {"schema": {"type": "object"}}}}
            }
        }
    }
    spec["paths"]["/v1/historical/candles"] = {
        "get": {
            "tags": ["Backtesting"],
            "summary": "Get Historical Candles for Expired Contracts",
            "description": "Fetches historical candles for expired F&O contracts.",
            "parameters": [
                {"name": "trading_symbol", "in": "query", "required": True, "schema": {"type": "string"}},
                {"name": "interval_in_minutes", "in": "query", "required": True, "schema": {"type": "string", "enum": interval_enum}},
                {"name": "start_time", "in": "query", "required": True, "schema": {"type": "string"}},
                {"name": "end_time", "in": "query", "required": True, "schema": {"type": "string"}},
                {"name": "exchange", "in": "query", "required": True, "schema": {"type": "string", "enum": exchange_enum}},
                {"name": "segment", "in": "query", "required": True, "schema": {"type": "string", "enum": segment_enum}}
            ],
            "responses": {
                "200": {"description": "Historical candle data", "content": {"application/json": {"schema": {"type": "object"}}}}
            }
        }
    }

    # 10. USER
    spec["components"]["schemas"]["UserProfileResponse"] = {
        "type": "object",
        "properties": {
            "status": {"type": "string"},
            "user_id": {"type": "string"},
            "demat_account_number": {"type": "string"},
            "active_segments": {"type": "array", "items": {"type": "string"}},
            "is_fno_enabled": {"type": "boolean"},
            "broker_name": {"type": "string", "example": "Groww"}
        }
    }
    spec["paths"]["/v1/user/detail"] = {
        "get": {
            "tags": ["User"],
            "summary": "Get User Profile",
            "description": "Fetches user account details, demat number, and active exchange segment permissions.",
            "responses": {
                "200": {"description": "User profile details", "content": {"application/json": {"schema": {"$ref": "#/components/schemas/UserProfileResponse"}}}}
            }
        }
    }

    return spec

if __name__ == "__main__":
    os.makedirs(os.path.dirname(output_file), exist_ok=True)
    openapi_spec = build_openapi()
    with open(output_file, "w") as f:
        json.dump(openapi_spec, f, indent=2)
    print(f"Generated complete OpenAPI spec at {output_file} ({len(openapi_spec['paths'])} paths, {len(openapi_spec['components']['schemas'])} schemas).")
