#!/usr/bin/env python3

import cgi
import html
import math
import requests
import os


def format_bytes(bytes_num, decimals=2):
    if bytes_num == 0:
        return "0 Bytes"
    units = ["Bytes", "KB", "MB", "GB", "TB", "PB", "EB", "ZB", "YB"]
    unit_index = 0
    while bytes_num >= 1024 and unit_index < len(units) - 1:
        bytes_num /= 1024
        unit_index += 1
    return f"{bytes_num:.{decimals}f} {units[unit_index]}"


print("Content-Type: text/html; charset=utf-8\n\n")

# GraphQL query with variables
query_by_ip = """
query  {
  peerTrace {
    torrentsByIP(ip: "1.1.1.1") {
      torrentTraces {
        torrent {
          infoHash
          name
          magnetUri
          size
        }
        lastSeenTime
      }
    }
  }
}
"""
query_by_infohash = """
query {
  peerTrace{
      peersByInfohash(infoHash: "16b2a97235506b42e5e4af2ebe9d86cc32d0414e"){
          peers{
              ip
              lastSeenTime
              infoHash
              location{
                  asn{
                      AutonomousSystemNumber
                      AutonomousSystemOrganization
                  }
                  country
                  city
                  latitude
                  longitude
              }
          }
      }
  }
}
"""


def print_html_header():
    # HTML template
    print(
        f"""
    <!DOCTYPE html>
    <html>
    <head>
    <title>GraphQL Torrent Data</title>
    <style type="text/css">
        .tftable {{
            font-size: 14px;
            color: #333333;
            width: 100%;
            border-width: 1px;
            border-color: #87ceeb;
            border-collapse: collapse;
            font-family: sans-serif;
        }}
        .tftable th {{
            font-size: 16px;
            background-color: #87ceeb;
            border-width: 1px;
            padding: 10px;
            border-style: solid;
            border-color: #87ceeb;
            text-align: left;
        }}
        .tftable tr {{
            background-color: #ffffff;
        }}
        .tftable td {{
            font-size: 14px;
            border-width: 1px;
            padding: 10px;
            border-style: solid;
            border-color: #87ceeb;
        }}
        .tftable tr:hover {{
            background-color: #e0ffff;
        }}
        @media (max-width: 768px) {{
            .tftable {{
                font-size: 12px;
            }}
            .tftable th, .tftable td {{
                padding: 5px;
            }}
        }}
        .info-hash-link {{
            color: blue;
            text-decoration: underline;
            cursor: pointer;
            width: 100%;
            display: block;
            overflow: hidden;
            text-overflow: ellipsis;
            white-space: nowrap;
        }}
    </style>
    </head>"""
    )


def do_query_by_ip(client_ip):
    try:
        
        response = requests.post(
            "http://127.0.0.1:3333/graphql",
            json={"query": query_by_ip.replace("1.1.1.1",client_ip)},
            headers={"Content-Type": "application/json"},
        )
        
        # print(response.request.body)
        response.raise_for_status()
        data = response.json()
    except Exception as e:
        print(f"<p style='color:red'>Error fetching data: {html.escape(str(e))}</p>")
        exit()

    # Check data structure
    try:
        torrent_traces = data["data"]["peerTrace"]["torrentsByIP"]["torrentTraces"]
    except (KeyError, TypeError):
        print("<p>No torrent data available</p>")
        exit()
    print_html_header()
    print(
       f"""
        <body>
        <h1>Torrent Data</h1>
        <p>Fetching data for IP: <span id="clientIP">{html.escape(client_ip)}</span></p>
        <table class="tftable">
        <thead>
            <tr>
            <th>Info Hash</th>
            <th>Name</th>
            <th>Size</th>
            <th>Last Seen Time</th>
            </tr>
        </thead>
        <tbody>
        """
    )

    for trace in torrent_traces:
        torrent = trace.get("torrent", {})
        info_hash = html.escape(torrent.get("infoHash", "")).upper()
        magnet_uri = html.escape(torrent.get("magnetUri", ""), quote=True)
        name = html.escape(torrent.get("name", ""))
        size = format_bytes(torrent.get("size", 0))
        last_seen = html.escape(trace.get("lastSeenTime", ""))

        print(
            f"""
        <tr>
        <td><a href="{magnet_uri}" class="info-hash-link" target="_blank">{info_hash}</a></td>
        <td>{name}</td>
        <td>{size}</td>
        <td>{last_seen}</td>
        </tr>
        """
        )

    print(
        """
    </tbody>
    </table>
    </body>
    </html>
    """
    )
    
def format_time(iso_time):
    try:
        dt = datetime.fromisoformat(iso_time.replace("Z", "+00:00"))
        return dt.strftime("%Y-%m-%d %H:%M:%S")
    except:
        return iso_time  # Return original if format error


def do_query_by_infosh(infohash):
    try:
        #print(query_by_infohash.replace("16b2a97235506b42e5e4af2ebe9d86cc32d0414e",infohash))
        response = requests.post(
            "http://127.0.0.1:3333/graphql",
            json={"query": query_by_infohash.replace("16b2a97235506b42e5e4af2ebe9d86cc32d0414e",infohash)},
            headers={"Content-Type": "application/json"},
        )
        # print(response.request.body)
        response.raise_for_status()
        data = response.json()
        # print(data)
    except Exception as e:
        print(f"<p style='color:red'>Error fetching data: {html.escape(str(e))}</p>")
        exit()

    # Check data structure
    try:
        peers = data["data"]["peerTrace"]["peersByInfohash"]["peers"]
    except (KeyError, TypeError):
        print(data)
        print("<p>No peer data available</p>")
        exit()

    print_html_header()
    print(
        f"""
    <body>
    <h1>Torrent Data</h1>
    <p>Showing peers for InfoHash: <span id="infoHash">{html.escape(infohash)}</span></p>
    <table class="tftable">
    <thead>
    <tr>
        <th>IP</th>
        <th>Last Seen Time</th>
        <th>ASN</th>
        <th>Country</th>
        <th>City</th>
        <th>Latitude</th>
        <th>Longitude</th>
    </tr>
    </thead>
    <tbody>
    """
    )

    for peer in peers:
        loc = peer.get("location", {})
        asn = loc.get("asn", {})

        # 数据清洗和格式化
        ip = html.escape(peer.get("ip", ""))
        last_seen = format_time(peer.get("lastSeenTime", ""))
        asn_number = html.escape(str(asn.get("AutonomousSystemNumber", "")))
        asn_org = html.escape(asn.get("AutonomousSystemOrganization", ""))
        country = html.escape(loc.get("country", ""))
        city = html.escape(loc.get("city", "N/A"))  # 处理空城市
        lat = f"{loc.get('latitude', 0):.4f}"
        lon = f"{loc.get('longitude', 0):.4f}"

        print(
            f"""
        <tr>
        <td>{ip}</td>
        <td>{last_seen}</td>
        <td class="asn-info">{asn_number} - {asn_org}</td>
        <td>{country}</td>
        <td>{city}</td>
        <td>{lat}</td>
        <td>{lon}</td>
        </tr>
        """
        )

    print(
        """
    </tbody>
    </table>
    </body>
    </html>
    """
    )


# print(os.environ)
# Get client IP from query parameters
form = cgi.FieldStorage()
client_ip = form.getvalue("ip", "")
infohash = form.getvalue("infohash", "")
# if client_ip and infohash are both blank, do query_by_ip
if client_ip == "":
    client_ip = os.environ["HTTP_CF_CONNECTING_IP"]
if infohash != "":
    do_query_by_infosh(infohash)
else:
    do_query_by_ip(client_ip=client_ip)
