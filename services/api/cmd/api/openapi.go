package main

import (
	"encoding/json"
	"net/http"
)

// phase2OpenAPI is deliberately kept in source so the published contract is
// available without a filesystem dependency in the container image.
var phase2OpenAPI = json.RawMessage(`{
  "openapi": "3.1.0",
  "info": {
    "title": "MEKU-In API",
    "version": "2.1.0",
    "description": "MEKU-In API with tenant-scoped catalogue, branch cash shifts and drawer reconciliation, server-priced cash checkout, pending full-value M-Pesa checkout initiation, provider reconciliation, guarded atomic settlement, procurement and supplier payables. Cash checkout requires an open user shift; M-Pesa remains outside drawer cash. Expense recording includes manager approval, immutable review/payment history and cash payment integration with active drawer shifts. Operational alert inboxes include branch stock thresholds, pending approvals and cash variances, with acknowledgement/resolution history and an append-only business audit ledger. Operational reports expose date-filtered sales, profit, stock value, staff and category mix aggregates, plus expense totals and cash-shift reconciliation history."
  },
  "servers": [
    {
      "url": "/api/v1"
    }
  ],
  "tags": [
    {
      "name": "Auth"
    },
    {
      "name": "Workspace"
    },
    {
      "name": "Staff"
    },
    {
      "name": "Catalog"
    },
    {
      "name": "Sales"
    },
    {
      "name": "Operations"
    }
  ],
  "paths": {
    "/settings/operational/": {
      "get": {
        "tags": [
          "Operations"
        ],
        "summary": "Get branch operational settings",
        "security": [
          {
            "cookieAuth": []
          }
        ],
        "responses": {
          "200": {
            "description": "Branch settings"
          },
          "403": {
            "description": "Permission denied"
          }
        }
      },
      "put": {
        "tags": [
          "Operations"
        ],
        "summary": "Update branch operational alert settings",
        "security": [
          {
            "cookieAuth": []
          }
        ],
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {
                "type": "object",
                "required": [
                  "cash_variance_alert_threshold_minor",
                  "low_stock_alerts_enabled",
                  "stock_out_alerts_enabled",
                  "expense_approval_alerts_enabled",
                  "supplier_invoice_alerts_enabled",
                  "cash_variance_alerts_enabled"
                ],
                "properties": {
                  "cash_variance_alert_threshold_minor": {
                    "type": "integer",
                    "minimum": 0
                  },
                  "low_stock_alerts_enabled": {
                    "type": "boolean"
                  },
                  "stock_out_alerts_enabled": {
                    "type": "boolean"
                  },
                  "expense_approval_alerts_enabled": {
                    "type": "boolean"
                  },
                  "supplier_invoice_alerts_enabled": {
                    "type": "boolean"
                  },
                  "cash_variance_alerts_enabled": {
                    "type": "boolean"
                  }
                }
              }
            }
          }
        },
        "responses": {
          "200": {
            "description": "Settings updated"
          },
          "400": {
            "description": "Invalid threshold"
          },
          "403": {
            "description": "Permission denied"
          }
        }
      }
    },
    "/payments/mpesa/": {
      "get": {
        "tags": [
          "Payments"
        ],
        "summary": "List M-Pesa attempts for the selected tenant branch",
        "description": "Returns a paginated operational queue with masked phone numbers. Callback payloads and provider credentials are never exposed.",
        "security": [
          {
            "cookieAuth": []
          }
        ],
        "parameters": [
          {
            "name": "status",
            "in": "query",
            "schema": {
              "type": "string"
            }
          },
          {
            "name": "limit",
            "in": "query",
            "schema": {
              "type": "integer",
              "minimum": 1,
              "maximum": 100
            }
          },
          {
            "name": "offset",
            "in": "query",
            "schema": {
              "type": "integer",
              "minimum": 0
            }
          }
        ],
        "responses": {
          "200": {
            "description": "M-Pesa attempt queue"
          },
          "400": {
            "description": "Invalid filter or pagination",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          }
        }
      }
    },
    "/payments/mpesa/{attemptID}/review-notes": {
      "post": {
        "tags": [
          "Payments"
        ],
        "summary": "Record an investigation note for an attempt under review",
        "description": "Audit-only action. Does not change payment, sale, or inventory state and cannot force settlement.",
        "security": [
          {
            "cookieAuth": []
          }
        ],
        "parameters": [
          {
            "name": "attemptID",
            "in": "path",
            "required": true,
            "schema": {
              "type": "string",
              "format": "uuid"
            }
          }
        ],
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {
                "type": "object",
                "required": [
                  "note"
                ],
                "properties": {
                  "note": {
                    "type": "string",
                    "maxLength": 500
                  }
                }
              }
            }
          }
        },
        "responses": {
          "201": {
            "description": "Review note recorded"
          },
          "404": {
            "description": "Attempt not found",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "409": {
            "description": "Attempt is not under review",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          }
        }
      }
    },
    "/payments/mpesa/{attemptID}/settle": {
      "post": {
        "summary": "Atomically settle a provider-confirmed M-Pesa sale",
        "security": [
          {
            "cookieAuth": []
          }
        ],
        "description": "Requires a provider-confirmed attempt, matching pending sale and payment allocation. Verifies the tender, completes the sale, deducts recipe-backed stock and writes the audit record in one tenant transaction. Idempotent after successful settlement.",
        "parameters": [
          {
            "name": "attemptID",
            "in": "path",
            "required": true,
            "schema": {
              "type": "string",
              "format": "uuid"
            }
          }
        ],
        "responses": {
          "200": {
            "description": "Sale settled or already settled"
          },
          "409": {
            "description": "Attempt is not eligible or stock/order conflict"
          },
          "404": {
            "description": "Attempt not found"
          }
        }
      }
    },
    "/payments/mpesa/{attemptID}/reconcile": {
      "post": {
        "summary": "Reconcile an M-Pesa STK attempt with Safaricom",
        "description": "Queries Safaricom and compares successful provider status with the stored callback amount, phone, receipt and request identifiers. Provider confirmation is not settlement: this endpoint does not complete the sale or deduct stock.",
        "security": [
          {
            "cookieAuth": []
          }
        ],
        "parameters": [
          {
            "name": "attemptID",
            "in": "path",
            "required": true,
            "schema": {
              "type": "string",
              "format": "uuid"
            }
          }
        ],
        "responses": {
          "200": {
            "description": "Provider state reconciled"
          },
          "404": {
            "description": "Attempt not found",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "409": {
            "description": "Attempt not ready or metadata mismatch",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "502": {
            "description": "Safaricom query failed",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "503": {
            "description": "Daraja is not configured",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          }
        }
      }
    },
    "/daraja/stk/callback": {
      "servers": [
        {
          "url": "/"
        }
      ],
      "post": {
        "tags": [
          "Payments"
        ],
        "summary": "Receive a Daraja STK Push callback for durable correlation only",
        "description": "The callback is stored idempotently against a matching provider attempt. It does not verify settlement, complete a sale, or deduct stock. A later provider status query is required.",
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {
                "type": "object"
              }
            }
          }
        },
        "responses": {
          "200": {
            "description": "Callback acknowledged"
          },
          "400": {
            "description": "Malformed callback"
          },
          "503": {
            "description": "Callback could not be persisted; provider retry is requested"
          }
        }
      }
    },
    "/auth/register": {
      "post": {
        "tags": [
          "Auth"
        ],
        "summary": "Create an owner workspace and send a required email verification code",
        "responses": {
          "201": {
            "description": "Workspace created; email verification required before login"
          },
          "400": {
            "description": "Invalid input",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "409": {
            "description": "Email already registered",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "default": {
            "description": "Structured API error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          }
        },
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {
                "$ref": "#/components/schemas/RegistrationRequest"
              }
            }
          }
        }
      }
    },
    "/auth/login": {
      "post": {
        "tags": [
          "Auth"
        ],
        "summary": "Authenticate a verified account; may require an authenticator code before session creation",
        "responses": {
          "200": {
            "description": "Authenticated or MFA challenge required"
          },
          "401": {
            "description": "Invalid credentials",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "429": {
            "description": "Rate limited",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "default": {
            "description": "Structured API error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          }
        }
      }
    },
    "/auth/me": {
      "get": {
        "tags": [
          "Auth"
        ],
        "summary": "Get the authenticated user",
        "responses": {
          "200": {
            "description": "Current user"
          },
          "401": {
            "description": "Unauthenticated",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "default": {
            "description": "Structured API error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          }
        },
        "security": [
          {
            "cookieAuth": []
          }
        ]
      }
    },
    "/auth/logout": {
      "post": {
        "tags": [
          "Auth"
        ],
        "summary": "Revoke the current session",
        "responses": {
          "204": {
            "description": "Session revoked"
          },
          "default": {
            "description": "Structured API error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          }
        },
        "security": [
          {
            "cookieAuth": []
          }
        ]
      }
    },
    "/workspace": {
      "get": {
        "tags": [
          "Workspace"
        ],
        "summary": "List workspaces available to the authenticated user",
        "responses": {
          "200": {
            "description": "Authorized workspaces"
          },
          "default": {
            "description": "Structured API error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          }
        },
        "security": [
          {
            "cookieAuth": []
          }
        ]
      }
    },
    "/tenant/select": {
      "post": {
        "tags": [
          "Workspace"
        ],
        "summary": "Select an authorized business and branch",
        "responses": {
          "200": {
            "description": "Workspace selected"
          },
          "403": {
            "description": "Membership or branch not authorized",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "default": {
            "description": "Structured API error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          }
        },
        "security": [
          {
            "cookieAuth": []
          }
        ],
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {
                "$ref": "#/components/schemas/TenantSelectionRequest"
              }
            }
          }
        }
      }
    },
    "/branches/": {
      "get": {
        "tags": [
          "Branches"
        ],
        "summary": "List branches in the selected business",
        "responses": {
          "200": {
            "description": "Branch directory"
          },
          "default": {
            "description": "Structured API error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          }
        },
        "security": [
          {
            "cookieAuth": []
          }
        ]
      },
      "post": {
        "tags": [
          "Branches"
        ],
        "summary": "Create a branch",
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {
                "type": "object",
                "required": [
                  "name"
                ],
                "properties": {
                  "name": {
                    "type": "string",
                    "minLength": 2,
                    "maxLength": 120
                  },
                  "phone": {
                    "type": "string",
                    "maxLength": 40
                  }
                }
              }
            }
          }
        },
        "responses": {
          "201": {
            "description": "Branch created"
          },
          "default": {
            "description": "Structured API error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          }
        },
        "security": [
          {
            "cookieAuth": []
          }
        ]
      }
    },
    "/branches/{branchID}": {
      "put": {
        "tags": [
          "Branches"
        ],
        "summary": "Update branch details or active status",
        "parameters": [
          {
            "name": "branchID",
            "in": "path",
            "required": true,
            "schema": {
              "type": "string",
              "format": "uuid"
            }
          }
        ],
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {
                "type": "object",
                "required": [
                  "name",
                  "is_active"
                ],
                "properties": {
                  "name": {
                    "type": "string",
                    "minLength": 2,
                    "maxLength": 120
                  },
                  "phone": {
                    "type": "string",
                    "maxLength": 40
                  },
                  "is_active": {
                    "type": "boolean"
                  }
                }
              }
            }
          }
        },
        "responses": {
          "200": {
            "description": "Branch updated"
          },
          "404": {
            "description": "Branch not found"
          },
          "default": {
            "description": "Structured API error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          }
        },
        "security": [
          {
            "cookieAuth": []
          }
        ]
      }
    },
    "/staff/": {
      "get": {
        "tags": [
          "Staff"
        ],
        "summary": "List staff memberships and branch assignments",
        "parameters": [
          {
            "name": "q",
            "in": "query",
            "schema": {
              "type": "string",
              "maxLength": 100
            }
          },
          {
            "name": "limit",
            "in": "query",
            "schema": {
              "type": "integer",
              "minimum": 1,
              "maximum": 100,
              "default": 50
            }
          },
          {
            "name": "offset",
            "in": "query",
            "schema": {
              "type": "integer",
              "minimum": 0,
              "default": 0
            }
          }
        ],
        "responses": {
          "200": {
            "description": "Staff directory"
          },
          "403": {
            "description": "Permission denied",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "default": {
            "description": "Structured API error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          }
        },
        "security": [
          {
            "cookieAuth": []
          }
        ]
      }
    },
    "/staff/profiles": {
      "post": {
        "tags": [
          "Staff"
        ],
        "summary": "Create a staff profile for an existing membership",
        "responses": {
          "201": {
            "description": "Profile created"
          },
          "409": {
            "description": "Profile already exists",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "default": {
            "description": "Structured API error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          }
        },
        "security": [
          {
            "cookieAuth": []
          }
        ],
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {
                "$ref": "#/components/schemas/StaffProfileRequest"
              }
            }
          }
        }
      }
    },
    "/staff/{membershipID}/branches": {
      "put": {
        "tags": [
          "Staff"
        ],
        "summary": "Replace staff branch assignments",
        "parameters": [
          {
            "name": "membershipID",
            "in": "path",
            "required": true,
            "schema": {
              "type": "string",
              "format": "uuid"
            }
          }
        ],
        "responses": {
          "200": {
            "description": "Assignments updated"
          },
          "422": {
            "description": "Invalid or cross-business branch",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "default": {
            "description": "Structured API error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          }
        },
        "security": [
          {
            "cookieAuth": []
          }
        ]
      }
    },
    "/staff/{membershipID}/role": {
      "put": {
        "tags": [
          "Staff"
        ],
        "summary": "Change a membership role while preserving the last active owner",
        "parameters": [
          {
            "name": "membershipID",
            "in": "path",
            "required": true,
            "schema": {
              "type": "string",
              "format": "uuid"
            }
          }
        ],
        "responses": {
          "200": {
            "description": "Role updated"
          },
          "409": {
            "description": "Cannot demote the last active owner",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "default": {
            "description": "Structured API error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          }
        },
        "security": [
          {
            "cookieAuth": []
          }
        ]
      }
    },
    "/staff/{membershipID}/manager-pin": {
      "put": {
        "tags": [
          "Staff"
        ],
        "summary": "Set a manager or owner PIN (bcrypt hash stored)",
        "parameters": [
          {
            "name": "membershipID",
            "in": "path",
            "required": true,
            "schema": {
              "type": "string",
              "format": "uuid"
            }
          }
        ],
        "responses": {
          "200": {
            "description": "PIN updated"
          },
          "422": {
            "description": "Membership role or profile is not eligible",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "default": {
            "description": "Structured API error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          }
        },
        "security": [
          {
            "cookieAuth": []
          }
        ]
      }
    },
    "/staff/authorize-action": {
      "post": {
        "tags": [
          "Staff"
        ],
        "summary": "Verify a named manager PIN and issue a two-minute, one-use action approval",
        "description": "Requires manager_membership_id, PIN, an allowed action and optional target_id. The approver must be a different active owner/manager in the selected business. Five failures within a 15-minute window lock that approver PIN for 15 minutes. Approval token is bound to requester, business, branch, action and target; sales.refund additionally binds target_amount_minor. Sensitive handlers consume it in their own transaction.",
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {
                "$ref": "#/components/schemas/ManagerAuthorizationRequest"
              }
            }
          }
        },
        "responses": {
          "200": {
            "description": "Successful response",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/ManagerAuthorizationResult"
                }
              }
            }
          },
          "403": {
            "description": "PIN invalid, locked, unavailable or approver is requester",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "400": {
            "description": "Invalid authorization request",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "default": {
            "description": "Structured API error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          }
        },
        "security": [
          {
            "cookieAuth": []
          }
        ]
      }
    },
    "/catalog/categories": {
      "get": {
        "tags": [
          "Catalog"
        ],
        "parameters": [
          {
            "name": "q",
            "in": "query",
            "schema": {
              "type": "string",
              "maxLength": 100
            }
          },
          {
            "name": "limit",
            "in": "query",
            "schema": {
              "type": "integer",
              "minimum": 1,
              "maximum": 100,
              "default": 50
            }
          },
          {
            "name": "offset",
            "in": "query",
            "schema": {
              "type": "integer",
              "minimum": 0,
              "maximum": 1000000,
              "default": 0
            }
          }
        ],
        "summary": "List categories",
        "responses": {
          "200": {
            "description": "Category list"
          },
          "default": {
            "description": "Structured API error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          }
        },
        "security": [
          {
            "cookieAuth": []
          }
        ]
      },
      "post": {
        "tags": [
          "Catalog"
        ],
        "summary": "Create category",
        "responses": {
          "201": {
            "description": "Category created"
          },
          "default": {
            "description": "Structured API error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          }
        },
        "security": [
          {
            "cookieAuth": []
          }
        ]
      }
    },
    "/catalog/categories/{id}": {
      "patch": {
        "tags": [
          "Catalog"
        ],
        "summary": "Update category",
        "parameters": [
          {
            "name": "id",
            "in": "path",
            "required": true,
            "schema": {
              "type": "string",
              "format": "uuid"
            }
          }
        ],
        "responses": {
          "200": {
            "description": "Category updated"
          },
          "default": {
            "description": "Structured API error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          }
        },
        "security": [
          {
            "cookieAuth": []
          }
        ]
      }
    },
    "/catalog/categories/{id}/deactivate": {
      "post": {
        "tags": [
          "Catalog"
        ],
        "summary": "Deactivate category without deleting history",
        "responses": {
          "200": {
            "description": "Category deactivated"
          },
          "default": {
            "description": "Structured API error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          }
        },
        "security": [
          {
            "cookieAuth": []
          }
        ]
      }
    },
    "/catalog/units": {
      "get": {
        "tags": [
          "Catalog"
        ],
        "parameters": [
          {
            "name": "q",
            "in": "query",
            "schema": {
              "type": "string",
              "maxLength": 100
            }
          },
          {
            "name": "limit",
            "in": "query",
            "schema": {
              "type": "integer",
              "minimum": 1,
              "maximum": 100,
              "default": 50
            }
          },
          {
            "name": "offset",
            "in": "query",
            "schema": {
              "type": "integer",
              "minimum": 0,
              "maximum": 1000000,
              "default": 0
            }
          }
        ],
        "summary": "List units of measure",
        "responses": {
          "200": {
            "description": "Units"
          },
          "default": {
            "description": "Structured API error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          }
        },
        "security": [
          {
            "cookieAuth": []
          }
        ]
      },
      "post": {
        "tags": [
          "Catalog"
        ],
        "summary": "Create unit of measure",
        "responses": {
          "201": {
            "description": "Unit created"
          },
          "default": {
            "description": "Structured API error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          }
        },
        "security": [
          {
            "cookieAuth": []
          }
        ],
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {
                "$ref": "#/components/schemas/UnitRequest"
              }
            }
          }
        }
      }
    },
    "/catalog/units/{id}/deactivate": {
      "post": {
        "tags": [
          "Catalog"
        ],
        "summary": "Deactivate unit",
        "responses": {
          "200": {
            "description": "Unit deactivated"
          },
          "default": {
            "description": "Structured API error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          }
        },
        "security": [
          {
            "cookieAuth": []
          }
        ]
      }
    },
    "/catalog/unit-conversions": {
      "get": {
        "tags": [
          "Catalog"
        ],
        "parameters": [
          {
            "name": "q",
            "in": "query",
            "schema": {
              "type": "string",
              "maxLength": 100
            }
          },
          {
            "name": "limit",
            "in": "query",
            "schema": {
              "type": "integer",
              "minimum": 1,
              "maximum": 100,
              "default": 50
            }
          },
          {
            "name": "offset",
            "in": "query",
            "schema": {
              "type": "integer",
              "minimum": 0,
              "maximum": 1000000,
              "default": 0
            }
          }
        ],
        "summary": "List directional unit conversions",
        "responses": {
          "200": {
            "description": "Conversions"
          },
          "default": {
            "description": "Structured API error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          }
        },
        "security": [
          {
            "cookieAuth": []
          }
        ]
      },
      "post": {
        "tags": [
          "Catalog"
        ],
        "summary": "Create directional unit conversion",
        "responses": {
          "201": {
            "description": "Conversion created"
          },
          "default": {
            "description": "Structured API error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          }
        },
        "security": [
          {
            "cookieAuth": []
          }
        ],
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {
                "$ref": "#/components/schemas/UnitConversionRequest"
              }
            }
          }
        }
      }
    },
    "/catalog/products": {
      "get": {
        "tags": [
          "Catalog"
        ],
        "summary": "List products",
        "parameters": [
          {
            "name": "q",
            "in": "query",
            "schema": {
              "type": "string",
              "maxLength": 100
            }
          },
          {
            "name": "limit",
            "in": "query",
            "schema": {
              "type": "integer",
              "minimum": 1,
              "maximum": 100,
              "default": 50
            }
          },
          {
            "name": "offset",
            "in": "query",
            "schema": {
              "type": "integer",
              "minimum": 0,
              "default": 0
            }
          }
        ],
        "responses": {
          "200": {
            "description": "Products"
          },
          "default": {
            "description": "Structured API error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          }
        },
        "security": [
          {
            "cookieAuth": []
          }
        ]
      },
      "post": {
        "tags": [
          "Catalog"
        ],
        "summary": "Create product",
        "responses": {
          "201": {
            "description": "Product created"
          },
          "default": {
            "description": "Structured API error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          }
        },
        "security": [
          {
            "cookieAuth": []
          }
        ],
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {
                "$ref": "#/components/schemas/ProductRequest"
              }
            }
          }
        }
      }
    },
    "/catalog/products/{id}": {
      "patch": {
        "tags": [
          "Catalog"
        ],
        "summary": "Update mutable product fields",
        "responses": {
          "200": {
            "description": "Product updated"
          },
          "default": {
            "description": "Structured API error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          }
        },
        "security": [
          {
            "cookieAuth": []
          }
        ]
      }
    },
    "/catalog/products/{id}/deactivate": {
      "post": {
        "tags": [
          "Catalog"
        ],
        "summary": "Deactivate product without deleting historical references",
        "responses": {
          "200": {
            "description": "Product deactivated"
          },
          "default": {
            "description": "Structured API error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          }
        },
        "security": [
          {
            "cookieAuth": []
          }
        ]
      }
    },
    "/catalog/suppliers": {
      "get": {
        "tags": [
          "Catalog"
        ],
        "parameters": [
          {
            "name": "q",
            "in": "query",
            "schema": {
              "type": "string",
              "maxLength": 100
            }
          },
          {
            "name": "limit",
            "in": "query",
            "schema": {
              "type": "integer",
              "minimum": 1,
              "maximum": 100,
              "default": 50
            }
          },
          {
            "name": "offset",
            "in": "query",
            "schema": {
              "type": "integer",
              "minimum": 0,
              "maximum": 1000000,
              "default": 0
            }
          }
        ],
        "summary": "List suppliers",
        "responses": {
          "200": {
            "description": "Suppliers"
          },
          "default": {
            "description": "Structured API error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          }
        },
        "security": [
          {
            "cookieAuth": []
          }
        ]
      },
      "post": {
        "tags": [
          "Catalog"
        ],
        "summary": "Create supplier",
        "responses": {
          "201": {
            "description": "Supplier created"
          },
          "default": {
            "description": "Structured API error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          }
        },
        "security": [
          {
            "cookieAuth": []
          }
        ]
      }
    },
    "/catalog/suppliers/{id}": {
      "patch": {
        "tags": [
          "Catalog"
        ],
        "summary": "Update supplier",
        "responses": {
          "200": {
            "description": "Supplier updated"
          },
          "default": {
            "description": "Structured API error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          }
        },
        "security": [
          {
            "cookieAuth": []
          }
        ]
      }
    },
    "/catalog/suppliers/{id}/deactivate": {
      "post": {
        "tags": [
          "Catalog"
        ],
        "summary": "Deactivate supplier",
        "responses": {
          "200": {
            "description": "Supplier deactivated"
          },
          "default": {
            "description": "Structured API error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          }
        },
        "security": [
          {
            "cookieAuth": []
          }
        ]
      }
    },
    "/catalog/tables": {
      "get": {
        "tags": [
          "Catalog"
        ],
        "parameters": [
          {
            "name": "q",
            "in": "query",
            "schema": {
              "type": "string",
              "maxLength": 100
            }
          },
          {
            "name": "limit",
            "in": "query",
            "schema": {
              "type": "integer",
              "minimum": 1,
              "maximum": 100,
              "default": 50
            }
          },
          {
            "name": "offset",
            "in": "query",
            "schema": {
              "type": "integer",
              "minimum": 0,
              "maximum": 1000000,
              "default": 0
            }
          }
        ],
        "summary": "List branch tables",
        "responses": {
          "200": {
            "description": "Tables"
          },
          "default": {
            "description": "Structured API error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          }
        },
        "security": [
          {
            "cookieAuth": []
          }
        ]
      },
      "post": {
        "tags": [
          "Catalog"
        ],
        "summary": "Create branch table",
        "responses": {
          "201": {
            "description": "Table created"
          },
          "default": {
            "description": "Structured API error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          }
        },
        "security": [
          {
            "cookieAuth": []
          }
        ]
      }
    },
    "/catalog/tables/{id}": {
      "patch": {
        "tags": [
          "Catalog"
        ],
        "summary": "Update branch table",
        "responses": {
          "200": {
            "description": "Table updated"
          },
          "default": {
            "description": "Structured API error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          }
        },
        "security": [
          {
            "cookieAuth": []
          }
        ]
      }
    },
    "/catalog/tables/{id}/deactivate": {
      "post": {
        "tags": [
          "Catalog"
        ],
        "summary": "Deactivate branch table",
        "responses": {
          "200": {
            "description": "Table deactivated"
          },
          "default": {
            "description": "Structured API error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          }
        },
        "security": [
          {
            "cookieAuth": []
          }
        ]
      }
    },
    "/catalog/recipes": {
      "get": {
        "tags": [
          "Catalog"
        ],
        "parameters": [
          {
            "name": "q",
            "in": "query",
            "schema": {
              "type": "string",
              "maxLength": 100
            }
          },
          {
            "name": "limit",
            "in": "query",
            "schema": {
              "type": "integer",
              "minimum": 1,
              "maximum": 100,
              "default": 50
            }
          },
          {
            "name": "offset",
            "in": "query",
            "schema": {
              "type": "integer",
              "minimum": 0,
              "maximum": 1000000,
              "default": 0
            }
          }
        ],
        "summary": "List recipes",
        "responses": {
          "200": {
            "description": "Recipes"
          },
          "default": {
            "description": "Structured API error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          }
        },
        "security": [
          {
            "cookieAuth": []
          }
        ]
      },
      "post": {
        "tags": [
          "Catalog"
        ],
        "summary": "Create recipe and ingredients atomically",
        "responses": {
          "201": {
            "description": "Recipe created"
          },
          "default": {
            "description": "Structured API error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          }
        },
        "security": [
          {
            "cookieAuth": []
          }
        ],
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {
                "$ref": "#/components/schemas/RecipeRequest"
              }
            }
          }
        }
      }
    },
    "/catalog/recipes/{id}/cost": {
      "get": {
        "tags": [
          "Catalog"
        ],
        "summary": "Calculate recipe cost and gross margin using current catalog ingredient costs",
        "parameters": [
          {
            "name": "id",
            "in": "path",
            "required": true,
            "schema": {
              "type": "string",
              "format": "uuid"
            }
          }
        ],
        "responses": {
          "200": {
            "description": "Successful response",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/RecipeCost"
                }
              }
            }
          },
          "422": {
            "description": "Required directional unit conversion missing",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "default": {
            "description": "Structured API error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          }
        },
        "security": [
          {
            "cookieAuth": []
          }
        ]
      }
    },
    "/staff/memberships": {
      "post": {
        "tags": [
          "Staff"
        ],
        "summary": "Add an existing registered user to the selected business and assign branches",
        "description": "The target account must register first. Membership and branch assignments are created atomically. Requires staff.manage.",
        "security": [
          {
            "cookieAuth": []
          }
        ],
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {
                "$ref": "#/components/schemas/StaffMembershipRequest"
              }
            }
          }
        },
        "responses": {
          "201": {
            "description": "Membership created",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/StaffMembership"
                }
              }
            }
          },
          "400": {
            "description": "Invalid membership or branch assignment",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "404": {
            "description": "Registered account not found",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "409": {
            "description": "User already belongs to this business",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "default": {
            "description": "Structured API error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          }
        }
      }
    },
    "/staff/invitations": {
      "post": {
        "tags": [
          "Staff"
        ],
        "summary": "Invite a staff member by email",
        "description": "Requires staff.manage. SMTP must be configured. The invitation token is never returned by the API; only its digest is stored. Invitations expire after 72 hours and include the selected active branches.",
        "security": [
          {
            "cookieAuth": []
          }
        ],
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {
                "$ref": "#/components/schemas/StaffInvitationRequest"
              }
            }
          }
        },
        "responses": {
          "201": {
            "description": "Invitation email submitted",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/StaffInvitation"
                }
              }
            }
          },
          "400": {
            "description": "Invalid invitation",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "403": {
            "description": "Permission denied",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "503": {
            "description": "Email delivery or persistence unavailable",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "default": {
            "description": "Structured API error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          }
        }
      }
    },
    "/auth/invitations/accept": {
      "post": {
        "tags": [
          "Auth",
          "Staff"
        ],
        "summary": "Accept a staff invitation",
        "description": "Requires an authenticated active account. The account email must exactly match the invited email. Acceptance is atomic and single-use.",
        "security": [
          {
            "cookieAuth": []
          }
        ],
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {
                "$ref": "#/components/schemas/AcceptStaffInvitationRequest"
              }
            }
          }
        },
        "responses": {
          "200": {
            "description": "Membership created and invitation accepted",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/AcceptedStaffInvitation"
                }
              }
            }
          },
          "400": {
            "description": "Malformed token",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "403": {
            "description": "Account email does not match invitation",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "409": {
            "description": "Membership already exists or branch unavailable",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "410": {
            "description": "Invitation expired, revoked, or already used",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "default": {
            "description": "Structured API error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          }
        }
      }
    },
    "/staff/invitations/{invitationID}/revoke": {
      "post": {
        "tags": [
          "Staff"
        ],
        "summary": "Revoke a pending staff invitation",
        "security": [
          {
            "cookieAuth": []
          }
        ],
        "parameters": [
          {
            "name": "invitationID",
            "in": "path",
            "required": true,
            "schema": {
              "type": "string",
              "format": "uuid"
            }
          }
        ],
        "responses": {
          "200": {
            "description": "Invitation revoked"
          },
          "400": {
            "description": "Invalid invitation ID",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "404": {
            "description": "Pending invitation not found",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "default": {
            "description": "Structured API error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          }
        }
      }
    },
    "/auth/invitations/register": {
      "post": {
        "tags": [
          "Auth",
          "Staff"
        ],
        "summary": "Register an account using an emailed staff invitation",
        "description": "Public same-origin endpoint. The invited email is read from the invitation token and cannot be supplied by the client. The link verifies control of the email address. Account creation does not create an extra owner workspace; the invitation must then be accepted.",
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {
                "$ref": "#/components/schemas/RegisterInvitedAccountRequest"
              }
            }
          }
        },
        "responses": {
          "201": {
            "description": "Account registered and session cookie issued",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/RegisteredInvitedAccount"
                }
              }
            }
          },
          "400": {
            "description": "Invalid registration details",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "409": {
            "description": "An account already exists for the invited email; sign in instead",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "410": {
            "description": "Invitation expired or revoked",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "default": {
            "description": "Structured API error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          }
        }
      }
    },
    "/sales/{saleID}/void": {
      "post": {
        "tags": [
          "Sales"
        ],
        "summary": "Void a completed sale using a target-bound manager approval",
        "description": "Requires sales.void. The approval token must be bound to this exact sale, branch, requester and sales.void action. Token consumption, sale state transition and audit event occur in the same transaction; a failed operation rolls back token consumption.",
        "security": [
          {
            "cookieAuth": []
          }
        ],
        "parameters": [
          {
            "name": "saleID",
            "in": "path",
            "required": true,
            "schema": {
              "type": "string",
              "format": "uuid"
            }
          }
        ],
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {
                "$ref": "#/components/schemas/VoidSaleRequest"
              }
            }
          }
        },
        "responses": {
          "200": {
            "description": "Sale voided",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/VoidedSale"
                }
              }
            }
          },
          "400": {
            "description": "Invalid sale ID, token or reason",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "403": {
            "description": "Valid manager approval required",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "404": {
            "description": "Sale not found in selected branch",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "409": {
            "description": "Sale is already voided or not voidable",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "default": {
            "description": "Structured API error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          }
        }
      }
    },
    "/sales/{saleID}/refunds": {
      "post": {
        "summary": "Record a manager-approved refund",
        "security": [
          {
            "cookieAuth": []
          }
        ],
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {
                "$ref": "#/components/schemas/RefundRequest"
              }
            }
          }
        },
        "responses": {
          "200": {
            "description": "Refund recorded; payment settlement is separate",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/RefundResult"
                }
              }
            }
          },
          "201": {
            "description": "Refund recorded; payment settlement is separate",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/RefundResult"
                }
              }
            }
          },
          "400": {
            "description": "Request, permission or state error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "403": {
            "description": "Request, permission or state error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "404": {
            "description": "Request, permission or state error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "409": {
            "description": "Request, permission or state error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "default": {
            "description": "Request, permission or state error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          }
        }
      }
    },
    "/inventory/valuation": {
      "get": {
        "summary": "Get branch stock value at moving weighted-average cost",
        "security": [
          {
            "cookieAuth": []
          }
        ],
        "responses": {
          "200": {
            "description": "Stock valuation KPIs"
          },
          "403": {
            "description": "Permission denied"
          }
        }
      }
    },
    "/settings/integrations": {
      "get": {
        "summary": "List non-secret per-business integration status and capabilities",
        "security": [
          {
            "cookieAuth": []
          }
        ],
        "responses": {
          "200": {
            "description": "Integration status and documented provider limitations"
          }
        }
      }
    },
    "/settings/integrations/{provider}": {
      "put": {
        "summary": "Update non-secret integration metadata and status; secrets remain deployment-managed",
        "security": [
          {
            "cookieAuth": []
          }
        ],
        "responses": {
          "200": {
            "description": "Integration metadata saved"
          },
          "403": {
            "description": "Owner permission required"
          },
          "409": {
            "description": "Provider is not configured for live operation"
          }
        }
      }
    },
    "/reports/summary": {
      "get": {
        "summary": "Get date-filtered sales, profit, stock value, staff or category-mix report",
        "security": [
          {
            "cookieAuth": []
          }
        ],
        "parameters": [
          {
            "name": "tab",
            "in": "query",
            "schema": {
              "type": "string",
              "enum": [
                "sales",
                "profit",
                "stock-value",
                "staff",
                "category-mix"
              ]
            }
          },
          {
            "name": "from",
            "in": "query",
            "schema": {
              "type": "string",
              "format": "date"
            }
          },
          {
            "name": "to",
            "in": "query",
            "schema": {
              "type": "string",
              "format": "date"
            }
          }
        ],
        "responses": {
          "200": {
            "description": "Paginated, branch-scoped report aggregates"
          },
          "403": {
            "description": "Permission denied"
          }
        }
      }
    },
    "/inventory/counts/": {
      "get": {
        "summary": "List branch inventory counts",
        "security": [
          {
            "cookieAuth": []
          }
        ],
        "responses": {
          "200": {
            "description": "Paginated count history"
          },
          "403": {
            "description": "Permission denied"
          }
        }
      },
      "post": {
        "summary": "Create a draft physical inventory count",
        "security": [
          {
            "cookieAuth": []
          }
        ],
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {
                "$ref": "#/components/schemas/InventoryCountCreateRequest"
              }
            }
          }
        },
        "responses": {
          "200": {
            "description": "Inventory count created",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/InventoryCountResult"
                }
              }
            }
          },
          "201": {
            "description": "Inventory count created",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/InventoryCountResult"
                }
              }
            }
          },
          "400": {
            "description": "Request, permission or state error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "403": {
            "description": "Request, permission or state error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "404": {
            "description": "Request, permission or state error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "409": {
            "description": "Request, permission or state error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "default": {
            "description": "Request, permission or state error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          }
        }
      }
    },
    "/inventory/counts/{countID}": {
      "get": {
        "summary": "Get a count and its variance details; blind counts hide expected values",
        "security": [
          {
            "cookieAuth": []
          }
        ],
        "responses": {
          "200": {
            "description": "Inventory count details"
          },
          "404": {
            "description": "Count not found"
          }
        }
      },
      "patch": {
        "summary": "Edit counted quantities on a draft inventory count",
        "security": [
          {
            "cookieAuth": []
          }
        ],
        "responses": {
          "200": {
            "description": "Draft updated"
          },
          "409": {
            "description": "Only draft counts can be edited"
          }
        }
      }
    },
    "/inventory/counts/{countID}/submit": {
      "post": {
        "summary": "Submit an editable draft inventory count for manager approval",
        "security": [
          {
            "cookieAuth": []
          }
        ],
        "responses": {
          "200": {
            "description": "Count submitted"
          },
          "409": {
            "description": "Count is not a draft"
          }
        }
      }
    },
    "/inventory/counts/{countID}/reconcile": {
      "post": {
        "summary": "Reconcile a count using target-bound manager approval",
        "security": [
          {
            "cookieAuth": []
          }
        ],
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {
                "$ref": "#/components/schemas/InventoryCountReconcileRequest"
              }
            }
          }
        },
        "responses": {
          "200": {
            "description": "Count reconciled and stock movements posted atomically",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/InventoryCountResult"
                }
              }
            }
          },
          "201": {
            "description": "Count reconciled and stock movements posted atomically",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/InventoryCountResult"
                }
              }
            }
          },
          "400": {
            "description": "Request, permission or state error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "403": {
            "description": "Request, permission or state error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "404": {
            "description": "Request, permission or state error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "409": {
            "description": "Request, permission or state error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "default": {
            "description": "Request, permission or state error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          }
        }
      }
    },
    "/sales/checkout": {
      "post": {
        "tags": [
          "Sales"
        ],
        "summary": "Create a sale and record its tender atomically",
        "description": "Requires pos.use and an Idempotency-Key header. Catalogue prices are recalculated server-side. Cash completes immediately only when the authenticated cashier has an open cash shift; the tender is attributed to that shift. One full-value KES M-Pesa tender creates a pending sale and durable dispatch-outbox item; a background worker submits STK Push after the checkout transaction commits. Recipe stock is deferred until provider-confirmed atomic settlement. M-Pesa currently requires phone_e164, whole-KES amount, no split rails and direct sale checkout (not a table order). Tax, service charge and discounts remain unchanged.",
        "security": [
          {
            "cookieAuth": []
          }
        ],
        "parameters": [
          {
            "name": "Idempotency-Key",
            "in": "header",
            "required": true,
            "schema": {
              "type": "string",
              "minLength": 8,
              "maxLength": 64
            }
          }
        ],
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {
                "type": "object",
                "required": [
                  "lines",
                  "payment_method"
                ],
                "properties": {
                  "payment_method": {
                    "type": "string",
                    "enum": [
                      "cash",
                      "mpesa"
                    ]
                  },
                  "payments": {
                    "type": "array",
                    "maxItems": 10,
                    "items": {
                      "type": "object",
                      "required": [
                        "method",
                        "amount_minor"
                      ],
                      "properties": {
                        "method": {
                          "type": "string",
                          "enum": [
                            "cash",
                            "mpesa"
                          ]
                        },
                        "amount_minor": {
                          "type": "integer",
                          "minimum": 1
                        },
                        "phone_e164": {
                          "type": "string",
                          "pattern": "^254[0-9]{9}$",
                          "description": "Required for M-Pesa; amount must be whole KES and equal the sale total."
                        }
                      }
                    }
                  },
                  "lines": {
                    "type": "array",
                    "minItems": 1,
                    "maxItems": 100,
                    "items": {
                      "type": "object",
                      "required": [
                        "product_id",
                        "quantity"
                      ],
                      "properties": {
                        "product_id": {
                          "type": "string",
                          "format": "uuid"
                        },
                        "quantity": {
                          "type": "integer",
                          "minimum": 1,
                          "maximum": 10000
                        }
                      }
                    }
                  }
                }
              }
            }
          }
        },
        "responses": {
          "201": {
            "description": "Cash sale created and completed with immutable line snapshots and payment record"
          },
          "202": {
            "description": "M-Pesa sale committed pending provider payment; STK Push is queued for post-commit dispatch"
          },
          "200": {
            "description": "Idempotent replay of the original checkout"
          },
          "400": {
            "description": "Invalid request or missing idempotency key",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "409": {
            "description": "Idempotency key reused with different request",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "default": {
            "description": "Structured API error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          }
        }
      }
    },
    "/sales/quote": {
      "post": {
        "tags": [
          "Sales"
        ],
        "summary": "Calculate a server-priced POS basket",
        "description": "Requires pos.use. Prices are read from active sellable catalogue products. Accepts whole-unit quantities and returns exact integer minor-unit totals. Tax, service charge and discounts are not applied until the business pricing policy is approved.",
        "security": [
          {
            "cookieAuth": []
          }
        ],
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {
                "$ref": "#/components/schemas/SaleQuoteRequest"
              }
            }
          }
        },
        "responses": {
          "200": {
            "description": "Server-priced basket quote",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/SaleQuote"
                }
              }
            }
          },
          "400": {
            "description": "Invalid basket",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "422": {
            "description": "Product is not sellable or amount is invalid",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "default": {
            "description": "Structured API error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          }
        }
      }
    },
    "/sales/orders": {
      "post": {
        "tags": [
          "Sales"
        ],
        "summary": "Create a POS order draft",
        "description": "Requires pos.use. Optional client_reference enables tenant/branch-scoped idempotent replay for offline outbox drafts. Reusing a reference with a different canonical payload returns 409. Prices are recalculated from the live catalogue; this endpoint creates an order draft only and does not create a sale, payment, revenue or stock movement.",
        "security": [
          {
            "cookieAuth": []
          }
        ],
        "responses": {
          "200": {
            "description": "Previously created order returned for an idempotent replay"
          },
          "201": {
            "description": "Order created"
          },
          "400": {
            "description": "Invalid order"
          },
          "409": {
            "description": "Client reference reused with a different payload"
          },
          "422": {
            "description": "Product or table unavailable"
          }
        }
      },
      "get": {
        "tags": [
          "Sales"
        ],
        "summary": "List open and held POS orders",
        "security": [
          {
            "cookieAuth": []
          }
        ],
        "responses": {
          "200": {
            "description": "Orders"
          }
        }
      }
    },
    "/sales/orders/{orderID}": {
      "get": {
        "tags": [
          "Sales"
        ],
        "summary": "Retrieve a POS order and its line snapshots",
        "security": [
          {
            "cookieAuth": []
          }
        ],
        "parameters": [
          {
            "name": "orderID",
            "in": "path",
            "required": true,
            "schema": {
              "type": "string",
              "format": "uuid"
            }
          }
        ],
        "responses": {
          "200": {
            "description": "Order"
          },
          "404": {
            "description": "Order not found"
          }
        }
      },
      "put": {
        "tags": [
          "Sales"
        ],
        "summary": "Replace lines on an open POS order using current server prices",
        "security": [
          {
            "cookieAuth": []
          }
        ],
        "parameters": [
          {
            "name": "orderID",
            "in": "path",
            "required": true,
            "schema": {
              "type": "string",
              "format": "uuid"
            }
          }
        ],
        "responses": {
          "200": {
            "description": "Order updated"
          },
          "404": {
            "description": "Order not found"
          },
          "409": {
            "description": "Only open orders can be edited"
          },
          "422": {
            "description": "Product unavailable or invalid total"
          }
        }
      }
    },
    "/sales/orders/{orderID}/reopen": {
      "post": {
        "tags": [
          "Sales"
        ],
        "summary": "Reopen a held POS order",
        "security": [
          {
            "cookieAuth": []
          }
        ],
        "parameters": [
          {
            "name": "orderID",
            "in": "path",
            "required": true,
            "schema": {
              "type": "string",
              "format": "uuid"
            }
          }
        ],
        "responses": {
          "200": {
            "description": "Order reopened"
          },
          "409": {
            "description": "Order state does not permit this transition"
          }
        }
      }
    },
    "/sales/orders/{orderID}/cancel": {
      "post": {
        "tags": [
          "Sales"
        ],
        "summary": "Cancel an open or held POS order",
        "security": [
          {
            "cookieAuth": []
          }
        ],
        "parameters": [
          {
            "name": "orderID",
            "in": "path",
            "required": true,
            "schema": {
              "type": "string",
              "format": "uuid"
            }
          }
        ],
        "responses": {
          "200": {
            "description": "Order cancelled"
          },
          "409": {
            "description": "Order state does not permit this transition"
          }
        }
      }
    },
    "/sales/orders/{orderID}/hold": {
      "post": {
        "tags": [
          "Sales"
        ],
        "summary": "Hold an open POS order",
        "security": [
          {
            "cookieAuth": []
          }
        ],
        "parameters": [
          {
            "name": "orderID",
            "in": "path",
            "required": true,
            "schema": {
              "type": "string",
              "format": "uuid"
            }
          }
        ],
        "responses": {
          "200": {
            "description": "Order held"
          },
          "409": {
            "description": "Order is not open"
          }
        }
      }
    },
    "/sales/orders/{orderID}/checkout": {
      "post": {
        "summary": "Check out a POS order",
        "description": "Checks out an open order through the same transactional cash checkout path. The order is locked, server prices are reloaded, recipe stock is consumed, and the order is linked to the completed sale atomically.",
        "parameters": [
          {
            "name": "orderID",
            "in": "path",
            "required": true,
            "schema": {
              "type": "string",
              "format": "uuid"
            }
          },
          {
            "name": "Idempotency-Key",
            "in": "header",
            "required": true,
            "schema": {
              "type": "string",
              "minLength": 8,
              "maxLength": 64
            }
          }
        ],
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {
                "type": "object",
                "required": [
                  "payment_method"
                ],
                "properties": {
                  "payment_method": {
                    "type": "string",
                    "enum": [
                      "cash"
                    ]
                  }
                }
              }
            }
          }
        },
        "responses": {
          "201": {
            "description": "Order checked out and linked to sale"
          },
          "200": {
            "description": "Idempotent replay"
          },
          "409": {
            "description": "Order conflict or insufficient stock"
          },
          "422": {
            "description": "Invalid checkout or stock conversion"
          }
        },
        "security": [
          {
            "cookieAuth": []
          }
        ]
      }
    },
    "/inventory/branches": {
      "get": {
        "summary": "List active branches assigned to the current member",
        "security": [
          {
            "cookieAuth": []
          }
        ],
        "responses": {
          "200": {
            "description": "Assigned inventory branches"
          },
          "403": {
            "description": "Permission denied"
          }
        }
      }
    },
    "/inventory/balances": {
      "get": {
        "summary": "List branch stock balances derived from the immutable movement ledger",
        "security": [
          {
            "cookieAuth": []
          }
        ],
        "responses": {
          "200": {
            "description": "Current stock balances"
          },
          "201": {
            "description": "Current stock balances"
          },
          "400": {
            "description": "Request, permission or state error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "403": {
            "description": "Request, permission or state error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "409": {
            "description": "Request, permission or state error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "default": {
            "description": "Request, permission or state error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          }
        }
      }
    },
    "/inventory/movements": {
      "get": {
        "summary": "List branch-scoped immutable inventory movements",
        "security": [
          {
            "cookieAuth": []
          }
        ],
        "responses": {
          "200": {
            "description": "Paginated inventory movement history"
          },
          "400": {
            "description": "Invalid pagination"
          },
          "403": {
            "description": "Permission denied"
          }
        }
      }
    },
    "/inventory/adjustments": {
      "post": {
        "summary": "Record a reasoned manual stock adjustment",
        "security": [
          {
            "cookieAuth": []
          }
        ],
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {
                "type": "object",
                "required": [
                  "product_id",
                  "quantity_delta",
                  "reason"
                ],
                "properties": {
                  "product_id": {
                    "type": "string",
                    "format": "uuid"
                  },
                  "quantity_delta": {
                    "type": "string",
                    "pattern": "^-?[0-9]+(\\.[0-9]{1,8})?$"
                  },
                  "reason": {
                    "type": "string",
                    "minLength": 5,
                    "maxLength": 300
                  }
                }
              }
            }
          }
        },
        "responses": {
          "201": {
            "description": "Adjustment movement recorded"
          },
          "400": {
            "description": "Invalid adjustment"
          },
          "403": {
            "description": "Permission denied"
          },
          "409": {
            "description": "Adjustment would create negative stock"
          }
        }
      }
    },
    "/inventory/losses/{kind}": {
      "post": {
        "summary": "Record wastage or breakage as immutable negative stock movements",
        "security": [
          {
            "cookieAuth": []
          }
        ],
        "responses": {
          "200": {
            "description": "Stock loss recorded"
          },
          "201": {
            "description": "Stock loss recorded"
          },
          "400": {
            "description": "Request, permission or state error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "403": {
            "description": "Request, permission or state error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "409": {
            "description": "Request, permission or state error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "default": {
            "description": "Request, permission or state error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          }
        },
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {
                "type": "object"
              }
            }
          }
        }
      }
    },
    "/inventory/reconciliation": {
      "get": {
        "summary": "Compare recipe-backed consumption with physical stock usage",
        "security": [
          {
            "cookieAuth": []
          }
        ],
        "parameters": [
          {
            "name": "from",
            "in": "query",
            "required": true,
            "schema": {
              "type": "string",
              "format": "date"
            }
          },
          {
            "name": "to",
            "in": "query",
            "required": true,
            "schema": {
              "type": "string",
              "format": "date"
            }
          }
        ],
        "responses": {
          "200": {
            "description": "Stock leakage reconciliation"
          },
          "400": {
            "description": "Invalid date range"
          }
        }
      }
    },
    "/inventory/transfers": {
      "post": {
        "summary": "Transfer stock atomically between assigned branches",
        "security": [
          {
            "cookieAuth": []
          }
        ],
        "responses": {
          "200": {
            "description": "Transfer movements recorded at both branches"
          },
          "201": {
            "description": "Transfer movements recorded at both branches"
          },
          "400": {
            "description": "Request, permission or state error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "403": {
            "description": "Request, permission or state error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "409": {
            "description": "Request, permission or state error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "default": {
            "description": "Request, permission or state error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          }
        },
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {
                "type": "object"
              }
            }
          }
        }
      }
    },
    "/shifts/current": {
      "get": {
        "summary": "Get the current user cash shift and live drawer expectation",
        "security": [
          {
            "cookieAuth": []
          }
        ],
        "responses": {
          "200": {
            "description": "Current cash shift",
            "content": {
              "application/json": {
                "schema": {
                  "type": "object"
                }
              }
            }
          },
          "201": {
            "description": "Current cash shift",
            "content": {
              "application/json": {
                "schema": {
                  "type": "object"
                }
              }
            }
          },
          "400": {
            "description": "Request, permission or state error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "403": {
            "description": "Request, permission or state error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "404": {
            "description": "Request, permission or state error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "409": {
            "description": "Request, permission or state error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "default": {
            "description": "Request, permission or state error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          }
        }
      }
    },
    "/shifts/": {
      "post": {
        "summary": "Open a branch cash shift with an opening float",
        "security": [
          {
            "cookieAuth": []
          }
        ],
        "responses": {
          "200": {
            "description": "Cash shift opened",
            "content": {
              "application/json": {
                "schema": {
                  "type": "object"
                }
              }
            }
          },
          "201": {
            "description": "Cash shift opened",
            "content": {
              "application/json": {
                "schema": {
                  "type": "object"
                }
              }
            }
          },
          "400": {
            "description": "Request, permission or state error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "403": {
            "description": "Request, permission or state error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "404": {
            "description": "Request, permission or state error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "409": {
            "description": "Request, permission or state error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "default": {
            "description": "Request, permission or state error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          }
        },
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {
                "$ref": "#/components/schemas/CashShiftOpenRequest"
              }
            }
          }
        }
      }
    },
    "/shifts/{shiftID}/close": {
      "post": {
        "summary": "Close and reconcile a cash shift against cash tenders and drawer movements",
        "security": [
          {
            "cookieAuth": []
          }
        ],
        "responses": {
          "200": {
            "description": "Cash shift closed",
            "content": {
              "application/json": {
                "schema": {
                  "type": "object"
                }
              }
            }
          },
          "201": {
            "description": "Cash shift closed",
            "content": {
              "application/json": {
                "schema": {
                  "type": "object"
                }
              }
            }
          },
          "400": {
            "description": "Request, permission or state error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "403": {
            "description": "Request, permission or state error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "404": {
            "description": "Request, permission or state error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "409": {
            "description": "Request, permission or state error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "default": {
            "description": "Request, permission or state error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          }
        },
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {
                "$ref": "#/components/schemas/CashShiftCloseRequest"
              }
            }
          }
        }
      }
    },
    "/shifts/{shiftID}/movements": {
      "post": {
        "summary": "Record a reasoned cash paid-in or paid-out movement",
        "security": [
          {
            "cookieAuth": []
          }
        ],
        "responses": {
          "200": {
            "description": "Cash movement recorded",
            "content": {
              "application/json": {
                "schema": {
                  "type": "object"
                }
              }
            }
          },
          "201": {
            "description": "Cash movement recorded",
            "content": {
              "application/json": {
                "schema": {
                  "type": "object"
                }
              }
            }
          },
          "400": {
            "description": "Request, permission or state error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "403": {
            "description": "Request, permission or state error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "404": {
            "description": "Request, permission or state error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "409": {
            "description": "Request, permission or state error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "default": {
            "description": "Request, permission or state error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          }
        },
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {
                "$ref": "#/components/schemas/CashShiftMovementRequest"
              }
            }
          }
        }
      }
    },
    "/shifts/{shiftID}/summary": {
      "get": {
        "summary": "Retrieve a closed shift summary for printing",
        "security": [
          {
            "cookieAuth": []
          }
        ],
        "responses": {
          "200": {
            "description": "Closed cash shift summary",
            "content": {
              "application/json": {
                "schema": {
                  "type": "object"
                }
              }
            }
          },
          "201": {
            "description": "Closed cash shift summary",
            "content": {
              "application/json": {
                "schema": {
                  "type": "object"
                }
              }
            }
          },
          "400": {
            "description": "Request, permission or state error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "403": {
            "description": "Request, permission or state error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "404": {
            "description": "Request, permission or state error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "409": {
            "description": "Request, permission or state error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "default": {
            "description": "Request, permission or state error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          }
        }
      }
    },
    "/expenses/categories": {
      "get": {
        "summary": "List expense categories",
        "security": [
          {
            "cookieAuth": []
          }
        ],
        "responses": {
          "200": {
            "description": "Success"
          },
          "400": {
            "description": "Error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "401": {
            "description": "Error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "403": {
            "description": "Error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "404": {
            "description": "Error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "409": {
            "description": "Error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "422": {
            "description": "Error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "500": {
            "description": "Error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          }
        }
      },
      "post": {
        "summary": "Create expense category",
        "security": [
          {
            "cookieAuth": []
          }
        ],
        "responses": {
          "200": {
            "description": "Success"
          },
          "400": {
            "description": "Error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "401": {
            "description": "Error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "403": {
            "description": "Error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "404": {
            "description": "Error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "409": {
            "description": "Error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "422": {
            "description": "Error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "500": {
            "description": "Error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          }
        },
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {
                "type": "object"
              }
            }
          }
        }
      }
    },
    "/expenses/": {
      "get": {
        "summary": "List branch expenses",
        "security": [
          {
            "cookieAuth": []
          }
        ],
        "responses": {
          "200": {
            "description": "Success"
          },
          "400": {
            "description": "Error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "401": {
            "description": "Error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "403": {
            "description": "Error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "404": {
            "description": "Error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "409": {
            "description": "Error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "422": {
            "description": "Error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "500": {
            "description": "Error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          }
        }
      },
      "post": {
        "summary": "Submit an expense for approval",
        "security": [
          {
            "cookieAuth": []
          }
        ],
        "responses": {
          "200": {
            "description": "Success"
          },
          "400": {
            "description": "Error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "401": {
            "description": "Error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "403": {
            "description": "Error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "404": {
            "description": "Error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "409": {
            "description": "Error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "422": {
            "description": "Error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "500": {
            "description": "Error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          }
        },
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {
                "type": "object"
              }
            }
          }
        }
      }
    },
    "/expenses/{expenseID}/review": {
      "post": {
        "summary": "Approve or reject an expense",
        "security": [
          {
            "cookieAuth": []
          }
        ],
        "responses": {
          "200": {
            "description": "Success"
          },
          "400": {
            "description": "Error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "401": {
            "description": "Error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "403": {
            "description": "Error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "404": {
            "description": "Error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "409": {
            "description": "Error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "422": {
            "description": "Error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "500": {
            "description": "Error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          }
        },
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {
                "type": "object"
              }
            }
          }
        }
      }
    },
    "/expenses/{expenseID}/pay": {
      "post": {
        "summary": "Pay an approved expense",
        "security": [
          {
            "cookieAuth": []
          }
        ],
        "responses": {
          "200": {
            "description": "Success"
          },
          "400": {
            "description": "Error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "401": {
            "description": "Error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "403": {
            "description": "Error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "404": {
            "description": "Error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "409": {
            "description": "Error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "422": {
            "description": "Error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "500": {
            "description": "Error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          }
        },
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {
                "type": "object"
              }
            }
          }
        }
      }
    },
    "/expenses/reports/summary": {
      "get": {
        "summary": "Summarise expenses by category, currency and status",
        "security": [
          {
            "cookieAuth": []
          }
        ],
        "responses": {
          "200": {
            "description": "Success"
          },
          "400": {
            "description": "Error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "401": {
            "description": "Error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "403": {
            "description": "Error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "404": {
            "description": "Error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "409": {
            "description": "Error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "422": {
            "description": "Error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "500": {
            "description": "Error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          }
        }
      }
    },
    "/expenses/{expenseID}/reviews": {
      "get": {
        "summary": "List append-only expense review decisions",
        "security": [
          {
            "cookieAuth": []
          }
        ],
        "responses": {
          "200": {
            "description": "Review event list"
          },
          "401": {
            "description": "Error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "403": {
            "description": "Error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "500": {
            "description": "Error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          }
        }
      }
    },
    "/alerts/": {
      "get": {
        "tags": [
          "Operations"
        ],
        "summary": "List and refresh operational alerts for the selected branch",
        "security": [
          {
            "cookieAuth": []
          }
        ],
        "responses": {
          "200": {
            "description": "Successful response",
            "content": {
              "application/json": {
                "schema": {
                  "type": "object"
                }
              }
            }
          },
          "400": {
            "description": "Request failed",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "401": {
            "description": "Request failed",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "403": {
            "description": "Request failed",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "500": {
            "description": "Request failed",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          }
        }
      }
    },
    "/alerts/{alertID}/action": {
      "post": {
        "tags": [
          "Operations"
        ],
        "summary": "Acknowledge or resolve an operational alert",
        "security": [
          {
            "cookieAuth": []
          }
        ],
        "responses": {
          "200": {
            "description": "Successful response",
            "content": {
              "application/json": {
                "schema": {
                  "type": "object"
                }
              }
            }
          },
          "400": {
            "description": "Request failed",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "401": {
            "description": "Request failed",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "403": {
            "description": "Request failed",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "500": {
            "description": "Request failed",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          }
        },
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {
                "type": "object",
                "properties": {
                  "action": {
                    "type": "string",
                    "enum": [
                      "acknowledge",
                      "resolve"
                    ]
                  },
                  "note": {
                    "type": "string",
                    "maxLength": 1000
                  }
                }
              }
            }
          }
        }
      }
    },
    "/audit/": {
      "get": {
        "tags": [
          "Operations"
        ],
        "summary": "List immutable audit ledger events for the business",
        "security": [
          {
            "cookieAuth": []
          }
        ],
        "responses": {
          "200": {
            "description": "Successful response",
            "content": {
              "application/json": {
                "schema": {
                  "type": "object"
                }
              }
            }
          },
          "400": {
            "description": "Request failed",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "401": {
            "description": "Request failed",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "403": {
            "description": "Request failed",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "500": {
            "description": "Request failed",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          }
        }
      }
    },
    "/shifts/history": {
      "get": {
        "tags": [
          "Operations"
        ],
        "summary": "List closed cash shifts for the selected date range",
        "security": [
          {
            "cookieAuth": []
          }
        ],
        "parameters": [
          {
            "name": "from",
            "in": "query",
            "schema": {
              "type": "string",
              "format": "date"
            }
          },
          {
            "name": "to",
            "in": "query",
            "schema": {
              "type": "string",
              "format": "date"
            }
          },
          {
            "name": "limit",
            "in": "query",
            "schema": {
              "type": "integer",
              "minimum": 1,
              "maximum": 100
            }
          }
        ],
        "responses": {
          "200": {
            "description": "Closed shift history, scoped to own shifts or branch by permission"
          },
          "400": {
            "description": "Invalid date range or limit"
          },
          "403": {
            "description": "Permission denied"
          }
        }
      }
    },
    "/reports/operational-summary": {
      "get": {
        "tags": [
          "Operations"
        ],
        "summary": "Get date-filtered operational expense and cash-shift summary",
        "security": [
          {
            "cookieAuth": []
          }
        ],
        "parameters": [
          {
            "name": "from",
            "in": "query",
            "schema": {
              "type": "string",
              "format": "date"
            }
          },
          {
            "name": "to",
            "in": "query",
            "schema": {
              "type": "string",
              "format": "date"
            }
          }
        ],
        "responses": {
          "200": {
            "description": "Expense totals grouped by currency and status, and closed shift count"
          },
          "400": {
            "description": "Invalid date range"
          },
          "403": {
            "description": "Permission denied"
          }
        }
      }
    },
    "/auth/email/verify": {
      "post": {
        "summary": "Verify registration email with a one-time code",
        "responses": {
          "200": {
            "description": "Success",
            "content": {
              "application/json": {
                "schema": {
                  "type": "object"
                }
              }
            }
          },
          "400": {
            "description": "Invalid request",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "500": {
            "description": "Server error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          }
        },
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {
                "type": "object"
              }
            }
          }
        }
      }
    },
    "/auth/email/resend": {
      "post": {
        "summary": "Resend registration email verification code",
        "responses": {
          "200": {
            "description": "Success",
            "content": {
              "application/json": {
                "schema": {
                  "type": "object"
                }
              }
            }
          },
          "400": {
            "description": "Invalid request",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "500": {
            "description": "Server error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          }
        },
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {
                "type": "object"
              }
            }
          }
        }
      }
    },
    "/auth/password/forgot": {
      "post": {
        "summary": "Request a password reset code",
        "responses": {
          "200": {
            "description": "Success",
            "content": {
              "application/json": {
                "schema": {
                  "type": "object"
                }
              }
            }
          },
          "400": {
            "description": "Invalid request",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "500": {
            "description": "Server error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          }
        },
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {
                "type": "object"
              }
            }
          }
        }
      }
    },
    "/auth/password/reset": {
      "post": {
        "summary": "Reset password using email code",
        "responses": {
          "200": {
            "description": "Success",
            "content": {
              "application/json": {
                "schema": {
                  "type": "object"
                }
              }
            }
          },
          "400": {
            "description": "Invalid request",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "500": {
            "description": "Server error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          }
        },
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {
                "type": "object"
              }
            }
          }
        }
      }
    },
    "/auth/mfa/verify": {
      "post": {
        "summary": "Complete login with authenticator code",
        "responses": {
          "200": {
            "description": "Success",
            "content": {
              "application/json": {
                "schema": {
                  "type": "object"
                }
              }
            }
          },
          "400": {
            "description": "Invalid request",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "500": {
            "description": "Server error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          }
        },
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {
                "type": "object"
              }
            }
          }
        }
      }
    },
    "/auth/google": {
      "post": {
        "summary": "Sign in with a Google Identity Services credential",
        "responses": {
          "200": {
            "description": "Success",
            "content": {
              "application/json": {
                "schema": {
                  "type": "object"
                }
              }
            }
          },
          "400": {
            "description": "Invalid request",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "500": {
            "description": "Server error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          }
        },
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {
                "type": "object"
              }
            }
          }
        }
      }
    },
    "/auth/mfa/setup": {
      "post": {
        "summary": "Create a pending authenticator secret",
        "responses": {
          "200": {
            "description": "Success",
            "content": {
              "application/json": {
                "schema": {
                  "type": "object"
                }
              }
            }
          },
          "400": {
            "description": "Invalid request",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "500": {
            "description": "Server error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          }
        },
        "security": [
          {
            "cookieAuth": []
          }
        ]
      }
    },
    "/auth/mfa/enable": {
      "post": {
        "summary": "Enable MFA after verifying an authenticator code",
        "responses": {
          "200": {
            "description": "Success",
            "content": {
              "application/json": {
                "schema": {
                  "type": "object"
                }
              }
            }
          },
          "400": {
            "description": "Invalid request",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "500": {
            "description": "Server error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          }
        },
        "security": [
          {
            "cookieAuth": []
          }
        ]
      }
    },
    "/auth/mfa/disable": {
      "post": {
        "summary": "Disable MFA after verifying an authenticator code",
        "responses": {
          "200": {
            "description": "Success",
            "content": {
              "application/json": {
                "schema": {
                  "type": "object"
                }
              }
            }
          },
          "400": {
            "description": "Invalid request",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "500": {
            "description": "Server error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          }
        },
        "security": [
          {
            "cookieAuth": []
          }
        ]
      }
    },
    "/auth/mfa/status": {
      "get": {
        "summary": "Get current user MFA status",
        "security": [
          {
            "cookieAuth": []
          }
        ],
        "responses": {
          "200": {
            "description": "MFA status",
            "content": {
              "application/json": {
                "schema": {
                  "type": "object"
                }
              }
            }
          },
          "500": {
            "description": "Server error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          }
        }
      }
    }
  },
  "components": {
    "securitySchemes": {
      "cookieAuth": {
        "type": "apiKey",
        "in": "cookie",
        "name": "mekuin_session"
      }
    },
    "schemas": {
      "Error": {
        "type": "object",
        "required": [
          "error"
        ],
        "properties": {
          "error": {
            "type": "string"
          },
          "detail": {
            "type": "string"
          }
        }
      },
      "RegistrationRequest": {
        "type": "object",
        "required": [
          "full_name",
          "email",
          "password",
          "business_name"
        ],
        "properties": {
          "full_name": {
            "type": "string",
            "minLength": 2,
            "maxLength": 160
          },
          "email": {
            "type": "string",
            "format": "email",
            "maxLength": 254
          },
          "password": {
            "type": "string",
            "minLength": 12,
            "maxLength": 72
          },
          "business_name": {
            "type": "string",
            "minLength": 2,
            "maxLength": 160
          },
          "branch_name": {
            "type": "string",
            "maxLength": 160
          },
          "phone": {
            "type": "string",
            "maxLength": 40
          }
        }
      },
      "TenantSelectionRequest": {
        "type": "object",
        "required": [
          "business_id",
          "branch_id"
        ],
        "properties": {
          "business_id": {
            "type": "string",
            "format": "uuid"
          },
          "branch_id": {
            "type": "string",
            "format": "uuid"
          }
        }
      },
      "StaffMembershipRequest": {
        "type": "object",
        "required": [
          "email",
          "role",
          "branch_ids"
        ],
        "properties": {
          "email": {
            "type": "string",
            "format": "email",
            "maxLength": 254
          },
          "role": {
            "type": "string",
            "enum": [
              "manager",
              "cashier",
              "bartender",
              "stock_controller"
            ]
          },
          "branch_ids": {
            "type": "array",
            "minItems": 1,
            "maxItems": 100,
            "uniqueItems": true,
            "items": {
              "type": "string",
              "format": "uuid"
            }
          }
        }
      },
      "StaffMembership": {
        "type": "object",
        "required": [
          "membership_id",
          "user_id",
          "role",
          "status"
        ],
        "properties": {
          "membership_id": {
            "type": "string",
            "format": "uuid"
          },
          "user_id": {
            "type": "string",
            "format": "uuid"
          },
          "role": {
            "type": "string"
          },
          "status": {
            "type": "string",
            "enum": [
              "active"
            ]
          }
        }
      },
      "StaffProfileRequest": {
        "type": "object",
        "required": [
          "membership_id",
          "display_name"
        ],
        "properties": {
          "membership_id": {
            "type": "string",
            "format": "uuid"
          },
          "display_name": {
            "type": "string",
            "minLength": 1,
            "maxLength": 160
          },
          "phone": {
            "type": "string",
            "maxLength": 40
          }
        }
      },
      "ManagerAuthorizationRequest": {
        "type": "object",
        "required": [
          "manager_membership_id",
          "pin",
          "action"
        ],
        "properties": {
          "manager_membership_id": {
            "type": "string",
            "format": "uuid"
          },
          "pin": {
            "type": "string",
            "pattern": "^[0-9]{4,8}$"
          },
          "action": {
            "type": "string",
            "enum": [
              "discount.approve",
              "sales.void",
              "sales.refund",
              "inventory.count.approve"
            ]
          },
          "target_id": {
            "type": "string",
            "format": "uuid"
          },
          "target_amount_minor": {
            "type": "integer",
            "minimum": 1,
            "description": "Required for sales.refund; binds manager approval to the exact refund amount in minor units."
          }
        }
      },
      "ManagerAuthorizationResult": {
        "type": "object",
        "required": [
          "authorized"
        ],
        "properties": {
          "authorized": {
            "type": "boolean"
          },
          "approval_token": {
            "type": "string",
            "pattern": "^[a-f0-9]{64}$"
          },
          "action": {
            "type": "string"
          },
          "expires_in_seconds": {
            "type": "integer"
          },
          "single_use": {
            "type": "boolean"
          },
          "error": {
            "type": "string"
          }
        }
      },
      "UnitRequest": {
        "type": "object",
        "required": [
          "code",
          "name",
          "dimension"
        ],
        "properties": {
          "code": {
            "type": "string",
            "maxLength": 24
          },
          "name": {
            "type": "string",
            "maxLength": 80
          },
          "dimension": {
            "type": "string",
            "enum": [
              "count",
              "mass",
              "volume"
            ]
          }
        }
      },
      "UnitConversionRequest": {
        "type": "object",
        "required": [
          "from_unit_id",
          "to_unit_id",
          "factor"
        ],
        "properties": {
          "from_unit_id": {
            "type": "string",
            "format": "uuid"
          },
          "to_unit_id": {
            "type": "string",
            "format": "uuid"
          },
          "factor": {
            "type": "string",
            "pattern": "^[0-9]+(\\.[0-9]+)?$"
          }
        }
      },
      "ProductRequest": {
        "type": "object",
        "required": [
          "base_unit_id",
          "sku",
          "name",
          "product_kind",
          "cost_price_minor",
          "sale_price_minor"
        ],
        "properties": {
          "category_id": {
            "type": [
              "string",
              "null"
            ],
            "format": "uuid"
          },
          "base_unit_id": {
            "type": "string",
            "format": "uuid"
          },
          "sku": {
            "type": "string",
            "maxLength": 80
          },
          "name": {
            "type": "string",
            "maxLength": 160
          },
          "description": {
            "type": "string"
          },
          "product_kind": {
            "type": "string",
            "enum": [
              "unit",
              "liquid"
            ]
          },
          "cost_price_minor": {
            "type": "integer",
            "minimum": 0
          },
          "sale_price_minor": {
            "type": "integer",
            "minimum": 0
          },
          "reorder_level": {
            "type": "string"
          },
          "bottle_size": {
            "type": "string"
          },
          "serving_size": {
            "type": "string"
          },
          "is_sellable": {
            "type": "boolean"
          }
        }
      },
      "RecipeRequest": {
        "type": "object",
        "required": [
          "output_product_id",
          "name",
          "ingredients"
        ],
        "properties": {
          "output_product_id": {
            "type": "string",
            "format": "uuid"
          },
          "name": {
            "type": "string",
            "maxLength": 160
          },
          "output_quantity": {
            "type": "string"
          },
          "instructions": {
            "type": "string"
          },
          "ingredients": {
            "type": "array",
            "minItems": 1,
            "maxItems": 100,
            "items": {
              "type": "object",
              "required": [
                "ingredient_product_id",
                "unit_id",
                "quantity"
              ],
              "properties": {
                "ingredient_product_id": {
                  "type": "string",
                  "format": "uuid"
                },
                "unit_id": {
                  "type": "string",
                  "format": "uuid"
                },
                "quantity": {
                  "type": "string"
                }
              }
            }
          }
        }
      },
      "RecipeCost": {
        "type": "object",
        "required": [
          "recipe_id",
          "name",
          "output_quantity",
          "output_sale_price_minor",
          "total_recipe_cost_minor",
          "cost_per_output_unit_minor",
          "gross_margin_minor",
          "cost_basis"
        ],
        "properties": {
          "recipe_id": {
            "type": "string",
            "format": "uuid"
          },
          "name": {
            "type": "string"
          },
          "output_quantity": {
            "type": "string"
          },
          "output_sale_price_minor": {
            "type": "integer"
          },
          "total_recipe_cost_minor": {
            "type": "string"
          },
          "cost_per_output_unit_minor": {
            "type": "string"
          },
          "gross_margin_minor": {
            "type": "string"
          },
          "gross_margin_percent": {
            "type": [
              "string",
              "null"
            ]
          },
          "cost_basis": {
            "type": "string"
          },
          "valuation_policy": {
            "type": "string"
          },
          "rounding": {
            "type": "string"
          }
        }
      },
      "StaffInvitationRequest": {
        "type": "object",
        "required": [
          "email",
          "role",
          "branch_ids"
        ],
        "properties": {
          "email": {
            "type": "string",
            "format": "email",
            "maxLength": 254
          },
          "role": {
            "type": "string",
            "enum": [
              "manager",
              "cashier",
              "bartender",
              "stock_controller"
            ]
          },
          "branch_ids": {
            "type": "array",
            "minItems": 1,
            "maxItems": 100,
            "uniqueItems": true,
            "items": {
              "type": "string",
              "format": "uuid"
            }
          }
        }
      },
      "StaffInvitation": {
        "type": "object",
        "required": [
          "invitation_id",
          "email",
          "role",
          "expires_in_hours",
          "delivery"
        ],
        "properties": {
          "invitation_id": {
            "type": "string",
            "format": "uuid"
          },
          "email": {
            "type": "string",
            "format": "email"
          },
          "role": {
            "type": "string"
          },
          "expires_in_hours": {
            "type": "integer",
            "const": 72
          },
          "delivery": {
            "type": "string",
            "const": "submitted"
          }
        }
      },
      "AcceptStaffInvitationRequest": {
        "type": "object",
        "required": [
          "token"
        ],
        "properties": {
          "token": {
            "type": "string",
            "pattern": "^[a-fA-F0-9]{64}$"
          }
        }
      },
      "AcceptedStaffInvitation": {
        "type": "object",
        "required": [
          "membership_id",
          "business_id",
          "role",
          "status"
        ],
        "properties": {
          "membership_id": {
            "type": "string",
            "format": "uuid"
          },
          "business_id": {
            "type": "string",
            "format": "uuid"
          },
          "role": {
            "type": "string"
          },
          "status": {
            "type": "string",
            "const": "active"
          }
        }
      },
      "RevokedStaffInvitation": {
        "type": "object",
        "required": [
          "invitation_id",
          "status"
        ],
        "properties": {
          "invitation_id": {
            "type": "string",
            "format": "uuid"
          },
          "status": {
            "type": "string",
            "const": "revoked"
          }
        }
      },
      "RegisterInvitedAccountRequest": {
        "type": "object",
        "required": [
          "token",
          "full_name",
          "password"
        ],
        "properties": {
          "token": {
            "type": "string",
            "pattern": "^[a-fA-F0-9]{64}$"
          },
          "full_name": {
            "type": "string",
            "minLength": 2,
            "maxLength": 120
          },
          "password": {
            "type": "string",
            "minLength": 12,
            "maxLength": 72
          }
        }
      },
      "RegisteredInvitedAccount": {
        "type": "object",
        "required": [
          "user_id",
          "email",
          "status"
        ],
        "properties": {
          "user_id": {
            "type": "string",
            "format": "uuid"
          },
          "email": {
            "type": "string",
            "format": "email"
          },
          "status": {
            "type": "string",
            "const": "registered"
          }
        }
      },
      "VoidSaleRequest": {
        "type": "object",
        "required": [
          "approval_token",
          "reason"
        ],
        "properties": {
          "approval_token": {
            "type": "string",
            "pattern": "^[a-fA-F0-9]{64}$"
          },
          "reason": {
            "type": "string",
            "minLength": 5,
            "maxLength": 500
          }
        }
      },
      "VoidedSale": {
        "type": "object",
        "required": [
          "sale_id",
          "status",
          "total_minor"
        ],
        "properties": {
          "sale_id": {
            "type": "string",
            "format": "uuid"
          },
          "status": {
            "type": "string",
            "const": "voided"
          },
          "total_minor": {
            "type": "integer",
            "minimum": 0
          }
        }
      },
      "RefundRequest": {
        "type": "object",
        "required": [
          "approval_token",
          "amount_minor",
          "reason"
        ],
        "properties": {
          "approval_token": {
            "type": "string",
            "minLength": 64,
            "maxLength": 64
          },
          "amount_minor": {
            "type": "integer",
            "minimum": 1
          },
          "reason": {
            "type": "string",
            "minLength": 5,
            "maxLength": 500
          }
        }
      },
      "RefundResult": {
        "type": "object",
        "required": [
          "refund_id",
          "sale_id",
          "amount_minor",
          "refunded_total_minor",
          "sale_total_minor",
          "status"
        ],
        "properties": {
          "refund_id": {
            "type": "string",
            "format": "uuid"
          },
          "sale_id": {
            "type": "string",
            "format": "uuid"
          },
          "amount_minor": {
            "type": "integer"
          },
          "refunded_total_minor": {
            "type": "integer"
          },
          "sale_total_minor": {
            "type": "integer"
          },
          "status": {
            "type": "string",
            "enum": [
              "recorded_pending_settlement"
            ]
          }
        }
      },
      "InventoryCountLineInput": {
        "type": "object",
        "required": [
          "product_id",
          "counted_quantity"
        ],
        "properties": {
          "product_id": {
            "type": "string",
            "format": "uuid"
          },
          "counted_quantity": {
            "type": "string",
            "pattern": "^[0-9]+(\\.[0-9]{1,8})?$"
          }
        }
      },
      "InventoryCountCreateRequest": {
        "type": "object",
        "required": [
          "lines"
        ],
        "properties": {
          "count_mode": {
            "type": "string",
            "enum": [
              "open",
              "blind"
            ],
            "default": "open"
          },
          "area": {
            "type": "string",
            "maxLength": 120
          },
          "lines": {
            "type": "array",
            "minItems": 1,
            "maxItems": 500,
            "items": {
              "$ref": "#/components/schemas/InventoryCountLineInput"
            }
          }
        }
      },
      "InventoryCountReconcileRequest": {
        "type": "object",
        "required": [
          "approval_token"
        ],
        "properties": {
          "approval_token": {
            "type": "string",
            "minLength": 64,
            "maxLength": 64
          }
        }
      },
      "InventoryCountResult": {
        "type": "object",
        "required": [
          "count_id",
          "status",
          "line_count"
        ],
        "properties": {
          "count_id": {
            "type": "string",
            "format": "uuid"
          },
          "status": {
            "type": "string",
            "enum": [
              "draft",
              "reconciled"
            ]
          },
          "line_count": {
            "type": "integer",
            "minimum": 0
          }
        }
      },
      "SaleQuoteRequest": {
        "type": "object",
        "additionalProperties": false,
        "required": [
          "lines"
        ],
        "properties": {
          "lines": {
            "type": "array",
            "minItems": 1,
            "maxItems": 100,
            "items": {
              "type": "object",
              "additionalProperties": false,
              "required": [
                "product_id",
                "quantity"
              ],
              "properties": {
                "product_id": {
                  "type": "string",
                  "format": "uuid"
                },
                "quantity": {
                  "type": "integer",
                  "minimum": 1,
                  "maximum": 10000
                }
              }
            }
          }
        }
      },
      "SaleQuote": {
        "type": "object",
        "required": [
          "currency",
          "lines",
          "totals",
          "calculation_policy"
        ],
        "properties": {
          "currency": {
            "type": "string",
            "minLength": 3,
            "maxLength": 3
          },
          "lines": {
            "type": "array",
            "items": {
              "type": "object",
              "required": [
                "product_id",
                "sku",
                "name",
                "quantity",
                "unit_price_minor",
                "line_total_minor"
              ],
              "properties": {
                "product_id": {
                  "type": "string",
                  "format": "uuid"
                },
                "sku": {
                  "type": "string"
                },
                "name": {
                  "type": "string"
                },
                "quantity": {
                  "type": "integer"
                },
                "unit_price_minor": {
                  "type": "integer",
                  "format": "int64"
                },
                "line_total_minor": {
                  "type": "integer",
                  "format": "int64"
                }
              }
            }
          },
          "totals": {
            "type": "object",
            "required": [
              "subtotal_minor",
              "discount_minor",
              "tax_minor",
              "service_charge_minor",
              "total_minor"
            ],
            "properties": {
              "subtotal_minor": {
                "type": "integer",
                "format": "int64"
              },
              "discount_minor": {
                "type": "integer",
                "format": "int64"
              },
              "tax_minor": {
                "type": "integer",
                "format": "int64"
              },
              "service_charge_minor": {
                "type": "integer",
                "format": "int64"
              },
              "total_minor": {
                "type": "integer",
                "format": "int64"
              }
            }
          },
          "calculation_policy": {
            "type": "string"
          }
        }
      },
      "CashShiftOpenRequest": {
        "type": "object",
        "additionalProperties": false,
        "required": [
          "opening_float_minor"
        ],
        "properties": {
          "opening_float_minor": {
            "type": "integer",
            "minimum": 0,
            "description": "Opening drawer float in business currency minor units."
          }
        }
      },
      "CashShiftCloseRequest": {
        "type": "object",
        "additionalProperties": false,
        "required": [
          "counted_cash_minor"
        ],
        "properties": {
          "counted_cash_minor": {
            "type": "integer",
            "minimum": 0,
            "description": "Physical counted drawer cash in minor units."
          }
        }
      },
      "CashShiftMovementRequest": {
        "type": "object",
        "additionalProperties": false,
        "required": [
          "direction",
          "amount_minor",
          "reason"
        ],
        "properties": {
          "direction": {
            "type": "string",
            "enum": [
              "paid_in",
              "paid_out"
            ]
          },
          "amount_minor": {
            "type": "integer",
            "minimum": 1
          },
          "reason": {
            "type": "string",
            "minLength": 5,
            "maxLength": 300
          }
        }
      }
    }
  }
}`)

func (s *server) openAPI(w http.ResponseWriter, _ *http.Request) {
	if !json.Valid(phase2OpenAPI) {
		http.Error(w, "OpenAPI document is invalid", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/vnd.oai.openapi+json;version=3.1")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(phase2OpenAPI)
}
