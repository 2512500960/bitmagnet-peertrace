import geoip2.database
import psycopg2
import psycopg2.extras
import pathlib
current_path = pathlib.Path(__file__).parent.resolve()
connection = psycopg2.connect(
            cursor_factory=psycopg2.extras.RealDictCursor,
            user="postgres",
            password="493255e4a1@",
            host="127.0.0.1",
            port="5432",
            dbname="bitmagnet",
        )



cursor = connection.cursor()
def main():
    #get_all_ip()
    reader = geoip2.database.Reader(str(current_path)+"/GeoLite2-City.mmdb")
    try:
        lines=open(str(current_path)+'/ipall.txt',"r").readlines()
        for line in lines:
            ip=line.split(",")[0]
            if ip.startswith("10."):
                continue
            try:
                response_city = reader.city(ip)
            except Exception as e:
                print(e)
                continue

            print("{} {}".format(ip, response_city.country.iso_code))
            if response_city.country.iso_code != "CN":

                cursor.execute(
                    "DELETE FROM public.peer_trace WHERE ip = %s", (ip,)
                )
                connection.commit()
                print(f"Deleted records for IP {ip}")

    except Exception as error:
        print(f"Error while connecting to PostgreSQL: {error}")
    finally:
        # 关闭数据库连接
        if connection:
            cursor.close()
            connection.close()
            print("PostgreSQL connection is closed.")

def get_all_ip():
    with open(str(current_path)+'/ipall.txt', 'w+') as f:
        limit = 100000
        offset = 0
        while True:
            # 执行查询
            cursor.execute(f"""
                SELECT DISTINCT
                    public.peer_trace.ip, last_seen_time
                FROM
                    public.peer_trace
                ORDER BY
                    public.peer_trace.last_seen_time ASC
                LIMIT {limit} OFFSET {offset}
            """)

            # 获取查询结果
            rows = cursor.fetchall()

            # 如果没有更多数据，退出循环
            if not rows:
                break

            # 将结果写入文件
            for row in rows:
                f.write(f"{row['ip']}, {row['last_seen_time']}\n")  # 假设你想将IP和最后看到的时间写入文件

            # 更新偏移量
            offset += limit
            
main()
